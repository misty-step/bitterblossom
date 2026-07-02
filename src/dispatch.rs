use std::path::{Path, PathBuf};
use std::time::{Duration, Instant};

use anyhow::{Context, Result};

use crate::budget;
use crate::harness;
use crate::ledger::{AttemptStats, Ledger, RunRow};
use crate::spec::{Plane, Task};
use crate::submit;
use crate::substrate::{self, WorkspacePlan, CARD_FILENAME};
const MAX_RETRIES: i64 = 2;
const DEFAULT_TIMEOUT_MINUTES: u64 = 60;
const LEASE_WAIT: Duration = Duration::from_secs(60);
const LEASE_POLL: Duration = Duration::from_millis(250);
enum AttemptOutcome {
    Success { stats: AttemptStats },
    Failure { phase_executed: bool, error: String },
}

pub fn dispatch_run(plane: &Plane, ledger: &mut Ledger, run_id: &str) -> Result<RunRow> {
    let run = ledger.run(run_id)?;
    if run.state != "pending" {
        return Ok(run);
    }
    let task = plane.task(&run.task)?;
    if let Some(source) = &task.source {
        ledger.set_run_config_source(run_id, &source.repo, &source.r#ref)?;
    }

    if let Some(v) = budget::pre_dispatch_check(plane, ledger, task)? {
        ledger.record_budget_event(Some(&task.name), v.kind, &v.detail)?;
        if v.kind == "max_runs_per_day" {
            ledger.park_task(&task.name, &v.detail)?;
        }
        ledger.transition(run_id, "blocked_budget", Some(&v.detail))?;
        crate::notify::notify(
            plane,
            ledger,
            "budget_blocked",
            &serde_json::json!({ "run_id": run_id, "task": task.name, "kind": v.kind, "detail": v.detail }),
        );
        return ledger.run(run_id);
    }
    if !ledger.try_transition(run_id, "running", None)? {
        return ledger.run(run_id);
    }
    ledger.set_run_agent(run_id, &task.agent_name, task.agent.version)?;
    let started = Instant::now();

    let mut attempt_n = ledger.attempt_count(run_id)?;
    loop {
        attempt_n += 1;
        let outcome = run_attempt(plane, ledger, run_id, task, attempt_n)?;
        match outcome {
            AttemptOutcome::Success { stats } => {
                ledger.finalize_run(
                    run_id,
                    stats.cost_usd,
                    started.elapsed().as_millis() as i64,
                )?;
                ledger.transition(run_id, "success", None)?;
                if let Some(v) = budget::post_run_check(task, stats.cost_usd) {
                    ledger.record_budget_event(Some(&task.name), v.kind, &v.detail)?;
                    ledger.park_task(&task.name, &v.detail)?;
                    crate::notify::notify(
                        plane,
                        ledger,
                        "budget_breach_parked",
                        &serde_json::json!({ "run_id": run_id, "task": task.name, "detail": v.detail }),
                    );
                }
                break;
            }
            AttemptOutcome::Failure {
                phase_executed: true,
                error,
            } => {
                ledger.finalize_run(run_id, None, started.elapsed().as_millis() as i64)?;
                ledger.transition(run_id, "failure", Some(&error))?;
                break;
            }
            AttemptOutcome::Failure {
                phase_executed: false,
                error,
            } => {
                if attempt_n - 1 < MAX_RETRIES {
                    ledger.record_event(run_id, "retry", Some(&error))?;
                    continue;
                }
                let payload = ledger.run_payload(run_id)?;
                let dl =
                    ledger.record_dead_letter(run_id, &task.name, payload.as_deref(), &error)?;
                ledger.finalize_run(run_id, None, started.elapsed().as_millis() as i64)?;
                ledger.transition(
                    run_id,
                    "failure",
                    Some(&format!("dead_letter:{dl} {error}")),
                )?;
                crate::notify::notify(
                    plane,
                    ledger,
                    "run_dead_lettered",
                    &serde_json::json!({ "run_id": run_id, "task": task.name, "dead_letter": dl, "error": error }),
                );
                break;
            }
        }
    }
    ledger.run(run_id)
}

fn run_attempt(
    plane: &Plane,
    ledger: &Ledger,
    run_id: &str,
    task: &Task,
    n: i64,
) -> Result<AttemptOutcome> {
    let host = task.host();
    let lease_wait = LEASE_WAIT.max(Duration::from_secs(
        60 * task
            .spec
            .budget
            .timeout_minutes
            .unwrap_or(DEFAULT_TIMEOUT_MINUTES),
    ));
    let lease_started = Instant::now();
    loop {
        if ledger.try_acquire_host_lease(&host, run_id)? {
            break;
        }
        if lease_started.elapsed() >= lease_wait {
            return Ok(AttemptOutcome::Failure {
                phase_executed: false,
                error: format!(
                    "host '{host}' lease held by run {:?}",
                    ledger.lease_holder(&host)?
                ),
            });
        }
        std::thread::sleep(LEASE_POLL);
    }

    let result = attempt_on_host(plane, ledger, run_id, task, n);
    ledger.release_host_lease(&host, run_id)?;
    result
}

fn attempt_on_host(
    plane: &Plane,
    ledger: &Ledger,
    run_id: &str,
    task: &Task,
    n: i64,
) -> Result<AttemptOutcome> {
    let attempt_dir = attempt_dir(plane, run_id, n);
    std::fs::create_dir_all(&attempt_dir)?;

    let attempt_id = ledger.create_attempt(
        run_id,
        n,
        &task.agent_name,
        task.agent.version,
        &task.agent.harness,
        &task.agent.model,
    )?;
    let artifact_dir = attempt_dir.to_string_lossy().into_owned();

    let fail = |phase_executed: bool, error: String| -> Result<AttemptOutcome> {
        ledger.finish_attempt(
            attempt_id,
            "failure",
            Some(&error),
            None,
            &AttemptStats::default(),
            Some(&artifact_dir),
        )?;
        Ok(AttemptOutcome::Failure {
            phase_executed,
            error,
        })
    };

    let substrate = match substrate::for_task(&task.spec.substrate) {
        Ok(s) => s,
        Err(e) => return fail(false, e.to_string()),
    };
    let mut session = match substrate.acquire(&task.host(), &attempt_dir) {
        Ok(s) => s,
        Err(e) => return fail(false, format!("acquire: {e:#}")),
    };
    let submission = match verdict_submission(ledger, run_id, task) {
        Ok(s) => s,
        Err(e) => {
            let _ = session.release();
            return fail(false, format!("verdict task: {e:#}"));
        }
    };

    let mut secrets = Vec::new();
    for name in &task.agent.secrets {
        let value = match crate::provider_keys::resolve_secret_for_task(plane, task, name) {
            Ok(Some(value)) => value,
            Ok(None) => {
                let Ok(value) = std::env::var(name) else {
                    let _ = session.release();
                    return fail(false, format!("secret env var '{name}' not set"));
                };
                value
            }
            Err(e) => {
                let _ = session.release();
                return fail(false, format!("provider key: {e:#}"));
            }
        };
        secrets.push((name.clone(), value));
    }
    let trigger = ledger.run(run_id)?;
    let plan = WorkspacePlan {
        repos: task.spec.workspace.repos.clone(),
        card: task.card.clone(),
        run_context: serde_json::json!({"run_id": run_id, "task": &task.name, "trigger": {"kind": trigger.trigger_kind, "idempotency_key": trigger.idempotency_key}, "agent": {"name": &task.agent_name, "version": task.agent.version, "role": &task.agent.role, "harness": &task.agent.harness, "model": &task.agent.model}, "substrate": &task.spec.substrate}).to_string(),
        payload: match (&submission, ledger.run_payload(run_id)?) {
            (Some(sub), Some(raw)) => {
                let mut v: serde_json::Value = serde_json::from_str(&raw)?;
                let obj = v.as_object_mut().context("verdict payload not an object")?;
                obj.insert("submission".into(), sub.id.clone().into());
                obj.insert("change".into(), sub.change_key.clone().into());
                obj.insert("rev".into(), sub.rev.clone().into());
                if let Some(ctx) = &sub.context {
                    obj.entry("context").or_insert_with(|| ctx.clone().into());
                }
                Some(v.to_string())
            }
            (_, payload) => payload,
        },
        report: submission.as_ref().and_then(|s| s.prior_report_json.clone()),
        pre_command: task.spec.pre_command.clone(),
        post_command: task.spec.post_command.clone(),
        marker: attempt_marker(attempt_id),
        workspace_name: task.name.clone(),
        checkpoint: task.spec.workspace.checkpoint.clone(),
        secrets,
        hermetic: matches!(task.agent.auth_class(), Ok(crate::spec::AuthClass::Api)),
    };
    if let Err(e) = session.prepare(&plan) {
        let _ = session.release();
        return fail(false, format!("prepare: {e:#}"));
    }
    ledger.set_attempt_phase(attempt_id, "prepared")?;
    ledger.record_progress(run_id, "phase:prepared")?;

    let cmd = match harness::build_command(&task.agent, &task.spec.budget) {
        Ok(c) => c,
        Err(e) => {
            let _ = session.release();
            return fail(false, format!("build_command: {e:#}"));
        }
    };

    ledger.set_attempt_phase(attempt_id, "executing")?;
    ledger.record_progress(run_id, "phase:executing")?;
    let timeout = Duration::from_secs(
        60 * task
            .spec
            .budget
            .timeout_minutes
            .unwrap_or(DEFAULT_TIMEOUT_MINUTES),
    );
    let card_prompt = format!(
        "Read {CARD_FILENAME} in this directory — it is your entire commission. Execute it.\n\n{}",
        task.card
    );
    let exec = match session.execute(&cmd, Some(&card_prompt), timeout) {
        Ok(r) => r,
        Err(e) => {
            let _ = session.release();
            return fail(true, format!("execute: {e:#}"));
        }
    };

    ledger.set_attempt_phase(attempt_id, "collecting")?;
    ledger.record_progress(run_id, "phase:collecting")?;
    session.write_artifact("stdout.txt", exec.stdout.as_bytes())?;
    session.write_artifact("stderr.txt", exec.stderr.as_bytes())?;

    if exec.timed_out {
        let _ = session.release();
        ledger.finish_attempt(
            attempt_id,
            "failure",
            Some("wall-clock timeout (killed)"),
            Some(exec.exit_code),
            &AttemptStats::default(),
            Some(&artifact_dir),
        )?;
        return Ok(AttemptOutcome::Failure {
            phase_executed: true,
            error: format!("timeout after {}s", timeout.as_secs()),
        });
    }
    if let (Some(sub), "command", Some(kind)) = (
        &submission,
        task.agent.harness.as_str(),
        task.spec.verdict.as_deref(),
    ) {
        let doc = if exec.exit_code == 0 {
            submit::VerdictDoc {
                verdict: "pass".into(),
                findings: vec![],
            }
        } else {
            let claim = format!("{kind} failed (non-zero exit)");
            submit::VerdictDoc {
                verdict: "blocking".into(),
                findings: vec![submit::Finding {
                    severity: "blocking".into(),
                    file: None,
                    line: None,
                    fingerprint: Some(submit::fingerprint(kind, None, &claim)),
                    claim,
                    evidence: Some(if exec.stderr.trim().is_empty() {
                        tail(exec.stdout.trim(), 2000)
                    } else {
                        tail(exec.stderr.trim(), 2000)
                    }),
                }],
            }
        };
        ledger.record_verdict(&sub.id, run_id, kind, &doc)?;
        session.release().context("release session")?;
        if let Some(error) = missing_artifact_error(&task.spec.required_artifacts, &attempt_dir) {
            ledger.finish_attempt(
                attempt_id,
                "failure",
                Some(&error),
                Some(exec.exit_code),
                &AttemptStats::default(),
                Some(&artifact_dir),
            )?;
            return Ok(AttemptOutcome::Failure {
                phase_executed: true,
                error,
            });
        }
        ledger.finish_attempt(
            attempt_id,
            "success",
            None,
            Some(exec.exit_code),
            &AttemptStats::default(),
            Some(&artifact_dir),
        )?;
        ledger.set_attempt_phase(attempt_id, "released")?;
        ledger.record_progress(run_id, "phase:released")?;
        return Ok(AttemptOutcome::Success {
            stats: AttemptStats::default(),
        });
    }

    if exec.exit_code != 0 {
        let _ = session.release();
        let error = format!(
            "harness exit {}: {}",
            exec.exit_code,
            truncate(exec.stderr.trim(), 500)
        );
        ledger.finish_attempt(
            attempt_id,
            "failure",
            Some(&error),
            Some(exec.exit_code),
            &AttemptStats::default(),
            Some(&artifact_dir),
        )?;
        return Ok(AttemptOutcome::Failure {
            phase_executed: true,
            error,
        });
    }

    let parsed = match harness::parse_output(&task.agent.harness, &exec.stdout) {
        Ok(p) => p,
        Err(e) => {
            let _ = session.release();
            let error = format!("unparseable harness output: {e:#}");
            ledger.finish_attempt(
                attempt_id,
                "failure",
                Some(&error),
                Some(exec.exit_code),
                &AttemptStats::default(),
                Some(&artifact_dir),
            )?;
            return Ok(AttemptOutcome::Failure {
                phase_executed: true,
                error,
            });
        }
    };
    session.write_artifact("result.md", parsed.result.as_bytes())?;
    if let (Some(sub), Some(kind)) = (&submission, task.spec.verdict.as_deref()) {
        match submit::parse_verdict(kind, &parsed.result) {
            Ok(mut doc) => {
                submit::enforce_fingerprints(&mut doc, kind, &ledger.known_fingerprints(sub)?);
                ledger.record_verdict(&sub.id, run_id, kind, &doc)?
            }
            Err(e) => {
                let _ = session.release();
                let error = format!("invalid verdict JSON: {e:#}");
                ledger.finish_attempt(
                    attempt_id,
                    "failure",
                    Some(&error),
                    Some(exec.exit_code),
                    &parsed.stats,
                    Some(&artifact_dir),
                )?;
                return Ok(AttemptOutcome::Failure {
                    phase_executed: true,
                    error,
                });
            }
        }
    }

    ledger.set_attempt_phase(attempt_id, "finalizing")?;
    ledger.record_progress(run_id, "phase:finalizing")?;
    if let Some(post) = &plan.post_command {
        let post_result = session.execute(
            &["sh".into(), "-c".into(), post.clone()],
            None,
            Duration::from_secs(600),
        );
        let post_error = match post_result {
            Ok(res) if res.exit_code == 0 => None,
            Ok(res) => Some(format!(
                "post_command exit {}: {}",
                res.exit_code,
                truncate(res.stderr.trim(), 500)
            )),
            Err(e) => Some(format!("post_command: {e:#}")),
        };
        if let Some(error) = post_error {
            let _ = session.release();
            ledger.finish_attempt(
                attempt_id,
                "failure",
                Some(&error),
                Some(exec.exit_code),
                &parsed.stats,
                Some(&artifact_dir),
            )?;
            return Ok(AttemptOutcome::Failure {
                phase_executed: true,
                error,
            });
        }
    }
    session.release().context("release session")?;
    if let Some(error) = missing_artifact_error(&task.spec.required_artifacts, &attempt_dir) {
        ledger.finish_attempt(
            attempt_id,
            "failure",
            Some(&error),
            Some(exec.exit_code),
            &parsed.stats,
            Some(&artifact_dir),
        )?;
        return Ok(AttemptOutcome::Failure {
            phase_executed: true,
            error,
        });
    }
    ledger.finish_attempt(
        attempt_id,
        "success",
        None,
        Some(exec.exit_code),
        &parsed.stats,
        Some(&artifact_dir),
    )?;
    ledger.set_attempt_phase(attempt_id, "released")?;
    ledger.record_progress(run_id, "phase:released")?;
    Ok(AttemptOutcome::Success {
        stats: parsed.stats,
    })
}
fn verdict_submission(
    ledger: &Ledger,
    run_id: &str,
    task: &Task,
) -> Result<Option<crate::submit::SubmissionRow>> {
    if task.spec.verdict.is_none() {
        return Ok(None);
    }
    let payload = ledger
        .run_payload(run_id)?
        .context("payload required (with a 'submission' field)")?;
    let v: serde_json::Value = serde_json::from_str(&payload).context("payload not JSON")?;
    let id = v
        .get("submission")
        .and_then(serde_json::Value::as_str)
        .context("payload has no 'submission' field")?;
    Ok(Some(ledger.submission(id)?))
}

pub fn attempt_marker(attempt_id: i64) -> String {
    format!("bb-attempt-{attempt_id}")
}

pub fn attempt_dir(plane: &Plane, run_id: &str, n: i64) -> PathBuf {
    plane
        .root
        .join(".bb/runs")
        .join(run_id)
        .join(format!("attempt-{n}"))
}

fn missing_artifact_error(required: &[String], artifact_dir: &Path) -> Option<String> {
    let missing: Vec<&String> = required
        .iter()
        .filter(|name| !artifact_dir.join(name).exists())
        .collect();
    if missing.is_empty() {
        return None;
    }
    let names: Vec<&str> = missing.iter().map(|s| s.as_str()).collect();
    Some(format!("missing required artifact: {}", names.join(", ")))
}
fn tail(s: &str, max: usize) -> String {
    if s.len() <= max {
        return s.to_string();
    }
    let mut start = s.len() - max;
    while !s.is_char_boundary(start) {
        start += 1;
    }
    format!("…{}", &s[start..])
}

fn truncate(s: &str, max: usize) -> String {
    if s.len() <= max {
        return s.to_string();
    }
    let mut end = max;
    while !s.is_char_boundary(end) {
        end -= 1;
    }
    format!("{}…", &s[..end])
}
