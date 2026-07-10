use std::path::{Path, PathBuf};
use std::time::{Duration, Instant};

use anyhow::{bail, Context, Result};

use crate::budget;
use crate::harness;
use crate::ledger::{AttemptStats, Ledger, RunRow};
use crate::spec::{Plane, Task, TaskBudget, TriggerSpec};
use crate::submit;
use crate::substrate::{self, ExecSnapshot, WorkspacePlan, CARD_FILENAME};
const MAX_RETRIES: i64 = 2;
/// bitterblossom-930: re-exported for callers that only need dispatch's
/// vocabulary; the substrate module owns the canonical definition since it
/// is the layer that collects the file (mirrors `REPORT_FILENAME`).
pub use crate::substrate::ASK_PACKET_FILENAME;
const DEFAULT_TIMEOUT_MINUTES: u64 = 60;
const LEASE_WAIT: Duration = Duration::from_secs(60);
const LEASE_POLL: Duration = Duration::from_millis(250);

/// Byte cap for an ad hoc dispatch prompt/brief, shared by `bb dispatch` and
/// the opt-in MCP `bb_dispatch` tool.
pub const DISPATCH_BRIEF_MAX_BYTES: u64 = 1_048_576;

/// Turn a free-text label into a safe git branch-slug fragment: lowercase
/// alphanumerics separated by single hyphens, falling back to `"dispatch"`
/// when nothing survives.
pub fn slugify_label(label: &str) -> String {
    let mut out = String::new();
    for ch in label.chars() {
        if ch.is_ascii_alphanumeric() {
            out.push(ch.to_ascii_lowercase());
        } else if !out.ends_with('-') {
            out.push('-');
        }
    }
    let out = out.trim_matches('-').to_string();
    if out.is_empty() {
        "dispatch".to_string()
    } else {
        out
    }
}

fn task_accepts_manual(task: &Task) -> bool {
    task.spec
        .triggers
        .iter()
        .any(|trigger| matches!(trigger, TriggerSpec::Manual))
}

/// Select the task an ad hoc dispatch (CLI `bb dispatch` or the opt-in MCP
/// `bb_dispatch` tool) should enqueue against: `BB_DISPATCH_TASK` when set,
/// else a manual `dispatch` task, else a manual `build` task, else the single
/// unambiguous manual task on the plane.
pub fn default_dispatch_task(plane: &Plane) -> Result<String> {
    if let Ok(task) = std::env::var("BB_DISPATCH_TASK") {
        let task_ref = plane
            .tasks
            .get(&task)
            .with_context(|| format!("BB_DISPATCH_TASK names unknown task '{task}'"))?;
        if !task_accepts_manual(task_ref) {
            bail!("BB_DISPATCH_TASK task '{task}' has no manual trigger");
        }
        return Ok(task);
    }

    for candidate in ["dispatch", "build"] {
        if let Some(task) = plane.tasks.get(candidate) {
            if task_accepts_manual(task) {
                return Ok(candidate.to_string());
            }
        }
    }

    let manual = plane
        .tasks
        .values()
        .filter(|task| task_accepts_manual(task))
        .map(|task| task.name.clone())
        .collect::<Vec<_>>();
    match manual.as_slice() {
        [task] => Ok(task.clone()),
        [] => bail!("no manual dispatch task found; add a `dispatch` or `build` task"),
        _ => bail!(
            "multiple manual tasks found ({}); set BB_DISPATCH_TASK",
            manual.join(", ")
        ),
    }
}

/// Build the canonical `bb.dispatch_job.v1` payload shared by the CLI
/// `bb dispatch` command and the opt-in MCP `bb_dispatch` tool -- one
/// payload shape, assembled in exactly one place, so the two entry points
/// can never drift. `repo` must already exist and be a directory; `prompt`
/// must already be within `DISPATCH_BRIEF_MAX_BYTES`; both are checked here,
/// before the caller ever touches the ledger.
pub fn build_dispatch_job_payload(
    repo: &Path,
    prompt: &str,
    model: Option<String>,
    label: String,
    branch_slug: String,
    base_ref: Option<String>,
) -> Result<String> {
    let repo = repo
        .canonicalize()
        .with_context(|| format!("repo path {}", repo.display()))?;
    if !repo.is_dir() {
        bail!("repo path {} is not a directory", repo.display());
    }
    let prompt_bytes = prompt.len() as u64;
    if prompt_bytes > DISPATCH_BRIEF_MAX_BYTES {
        bail!("prompt is {prompt_bytes} bytes; max is {DISPATCH_BRIEF_MAX_BYTES}");
    }
    let mut payload = serde_json::json!({
        "schema_version": "bb.dispatch_job.v1",
        "repo": repo.to_string_lossy(),
        "prompt": prompt,
        "model": model,
        "label": label,
        "branch_slug": branch_slug,
    });
    if let Some(base_ref) = base_ref {
        payload["base_ref"] = serde_json::Value::String(base_ref);
    }
    Ok(payload.to_string())
}

/// Deterministic idempotency key for ad hoc dispatch de-dup: the same
/// `(repo, label, branch_slug, base_ref)` tuple always derives the same key,
/// so `Ledger::ingest`'s existing `(task, idempotency_key)` uniqueness
/// refuses a repeat dispatch of the same job -- it returns the original run
/// with `duplicate: true` instead of fanning out a second one. The explicit
/// force path is simply not calling this (pass `idempotency_key: None` to
/// `Ledger::ingest` instead), which always mints a fresh run.
pub fn dispatch_idempotency_key(
    repo: &str,
    label: &str,
    branch_slug: &str,
    base_ref: Option<&str>,
) -> String {
    use sha2::{Digest, Sha256};
    let mut h = Sha256::new();
    h.update(repo.as_bytes());
    h.update(b"\0");
    h.update(label.as_bytes());
    h.update(b"\0");
    h.update(branch_slug.as_bytes());
    h.update(b"\0");
    h.update(base_ref.unwrap_or_default().as_bytes());
    let digest = format!("{:x}", h.finalize());
    format!("mcp-dispatch:{branch_slug}:{}", &digest[..16])
}
enum AttemptOutcome {
    Success {
        stats: AttemptStats,
    },
    Failure {
        phase_executed: bool,
        error: String,
        stats: AttemptStats,
        cap_breach: Option<InFlightCapBreach>,
    },
    /// bitterblossom-930: the attempt wrote `ASK_PACKET.json` and stopped
    /// cleanly instead of finishing. Terminal, like Success -- never retries.
    Parked {
        stats: AttemptStats,
    },
}

#[derive(Clone, Debug)]
enum InFlightCapBreach {
    Cost {
        observed_cost_usd: f64,
        cap_usd: f64,
        policy: String,
    },
    Policy {
        cap_kind: String,
        observed: u64,
        cap: u64,
        policy: String,
    },
}

pub fn dispatch_run(plane: &Plane, ledger: &mut Ledger, run_id: &str) -> Result<RunRow> {
    let run = ledger.run(run_id)?;
    if run.state != "pending" {
        return Ok(run);
    }
    let base_task = plane.task(&run.task)?;
    let model_override = dispatch_model_override(ledger.run_payload(run_id)?)?;
    let overridden_task;
    let task = if let Some(model) = model_override {
        overridden_task = {
            let mut task = base_task.clone();
            task.agent.model = model;
            task
        };
        &overridden_task
    } else {
        base_task
    };
    if let Some(source) = &task.source {
        ledger.set_run_config_source(run_id, &source.repo, &source.r#ref)?;
    }

    match budget::admit_dispatch(plane, ledger, task, run_id)? {
        budget::DispatchAdmission::Running => {}
        budget::DispatchAdmission::Blocked(v) => {
            // Cost governor slice 1 (bitterblossom-960), escalate-once:
            // `admit_dispatch` already recorded this exact (task, kind)
            // budget event before returning, so a count of 1 here means
            // this is the first breach of this kind today. Every later
            // same-day, same-kind trigger (webhook/cron redelivery hitting
            // an already-blocked or already-parked task) is a grind
            // repeat -- still a real `blocked_budget` run row for audit,
            // but not a repeat notification.
            if ledger.budget_events_today_count(&task.name, v.kind)? <= 1 {
                crate::notify::notify(
                    plane,
                    ledger,
                    "budget_blocked",
                    &serde_json::json!({ "run_id": run_id, "task": task.name, "kind": v.kind, "detail": v.detail }),
                );
            }
            return ledger.run(run_id);
        }
        budget::DispatchAdmission::NotPending => {
            return ledger.run(run_id);
        }
    }
    ledger.set_run_agent(run_id, &task.agent_name, task.agent.version)?;
    if task.roster.agent.is_some() || task.roster.brief.is_some() {
        ledger.record_event(
            run_id,
            "roster_provenance",
            Some(&serde_json::to_string(&task.roster)?),
        )?;
    }
    crate::glass::post_dispatched(
        plane,
        ledger,
        run_id,
        &task.name,
        &task.agent_name,
        &task.agent.harness,
        &task.agent.model,
    );
    let started = Instant::now();

    let mut attempt_n = ledger.attempt_count(run_id)?;
    loop {
        attempt_n += 1;
        let outcome = run_attempt(plane, ledger, run_id, task, attempt_n)?;
        match outcome {
            AttemptOutcome::Success { stats } => {
                let duration_ms = started.elapsed().as_millis() as i64;
                ledger.finalize_run(run_id, stats.cost_usd, duration_ms)?;
                ledger.transition(run_id, "success", None)?;
                crate::glass::post_completed(
                    plane,
                    ledger,
                    run_id,
                    &task.name,
                    &task.agent_name,
                    "success",
                    stats.cost_usd,
                    Some(duration_ms),
                );
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
            AttemptOutcome::Parked { stats } => {
                let duration_ms = started.elapsed().as_millis() as i64;
                ledger.finalize_run(run_id, stats.cost_usd, duration_ms)?;
                ledger.transition(run_id, "parked_on_ask", None)?;
                crate::glass::post_completed(
                    plane,
                    ledger,
                    run_id,
                    &task.name,
                    &task.agent_name,
                    "parked_on_ask",
                    stats.cost_usd,
                    Some(duration_ms),
                );
                break;
            }
            AttemptOutcome::Failure {
                phase_executed: true,
                error,
                stats,
                cap_breach,
            } => {
                ledger.finalize_run(
                    run_id,
                    stats.cost_usd,
                    started.elapsed().as_millis() as i64,
                )?;
                if let Some(breach) = cap_breach {
                    match breach {
                        InFlightCapBreach::Cost {
                            observed_cost_usd,
                            cap_usd,
                            policy,
                        } => {
                            let (state, event, budget_kind, guard_kind) = if policy == "quarantine"
                            {
                                (
                                    "awaiting_recovery",
                                    "run_in_flight_cap_quarantined",
                                    "in_flight_cap_quarantined",
                                    "in_flight_cap_quarantined",
                                )
                            } else {
                                (
                                    "failure",
                                    "run_in_flight_cap_killed",
                                    "in_flight_cap_killed",
                                    "in_flight_cap_killed",
                                )
                            };
                            ledger.transition(run_id, state, Some(&error))?;
                            ledger.record_budget_event(Some(&task.name), budget_kind, &error)?;
                            ledger.record_guard_event(guard_kind, Some(&task.name), &error, 1)?;
                            crate::notify::notify(
                                plane,
                                ledger,
                                event,
                                &serde_json::json!({
                                    "run_id": run_id,
                                    "task": task.name,
                                    "agent": task.agent_name,
                                    "observed_cost_usd": observed_cost_usd,
                                    "cap_usd": cap_usd,
                                    "policy": policy,
                                    "detail": error,
                                }),
                            );
                        }
                        InFlightCapBreach::Policy {
                            cap_kind,
                            observed,
                            cap,
                            policy,
                        } => {
                            let (state, event, guard_kind) = if policy == "quarantine" {
                                (
                                    "awaiting_recovery",
                                    "run_policy_cap_quarantined",
                                    "policy_cap_quarantined",
                                )
                            } else {
                                ("failure", "run_policy_cap_killed", "policy_cap_killed")
                            };
                            ledger.transition(run_id, state, Some(&error))?;
                            ledger.record_guard_event(guard_kind, Some(&task.name), &error, 1)?;
                            crate::notify::notify(
                                plane,
                                ledger,
                                event,
                                &serde_json::json!({
                                    "run_id": run_id,
                                    "task": task.name,
                                    "agent": task.agent_name,
                                    "cap_kind": cap_kind,
                                    "observed": observed,
                                    "cap": cap,
                                    "policy": policy,
                                    "detail": error,
                                }),
                            );
                        }
                    }
                } else {
                    ledger.transition(run_id, "failure", Some(&error))?;
                }
                let final_state = ledger.run(run_id)?.state;
                crate::glass::post_completed(
                    plane,
                    ledger,
                    run_id,
                    &task.name,
                    &task.agent_name,
                    &final_state,
                    stats.cost_usd,
                    Some(started.elapsed().as_millis() as i64),
                );
                break;
            }
            AttemptOutcome::Failure {
                phase_executed: false,
                error,
                ..
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
                crate::glass::post_completed(
                    plane,
                    ledger,
                    run_id,
                    &task.name,
                    &task.agent_name,
                    "failure",
                    None,
                    Some(started.elapsed().as_millis() as i64),
                );
                break;
            }
        }
    }
    ledger.run(run_id)
}

fn dispatch_model_override(payload: Option<String>) -> Result<Option<String>> {
    let Some(raw) = payload else {
        return Ok(None);
    };
    let Ok(value) = serde_json::from_str::<serde_json::Value>(&raw) else {
        return Ok(None);
    };
    if value.get("schema_version").and_then(|v| v.as_str()) != Some("bb.dispatch_job.v1") {
        return Ok(None);
    }
    match value.get("model") {
        Some(serde_json::Value::String(model)) if !model.trim().is_empty() => {
            Ok(Some(model.trim().to_string()))
        }
        _ => Ok(None),
    }
}

fn run_attempt(
    plane: &Plane,
    ledger: &Ledger,
    run_id: &str,
    task: &Task,
    n: i64,
) -> Result<AttemptOutcome> {
    let host = task.host();
    let effective_budget = effective_budget(task);
    let lease_wait = LEASE_WAIT.max(Duration::from_secs(
        60 * effective_budget
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
                stats: AttemptStats::default(),
                cap_breach: None,
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
            stats: AttemptStats::default(),
            cap_breach: None,
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
    // Backlog 925: an optional secret that cannot be resolved degrades the
    // run instead of dead-lettering it -- it is simply absent from the
    // workload's env, and the card's own contract is responsible for
    // behaving sanely without it (e.g. a report-only agent still writes a
    // blocked/degraded report rather than crashing). A provider-key minting
    // error is a real misconfiguration, not an absent credential, so that
    // still fails the run the same as a required secret would.
    for name in &task.agent.optional_secrets {
        match crate::provider_keys::resolve_secret_for_task(plane, task, name) {
            Ok(Some(value)) => secrets.push((name.clone(), value)),
            Ok(None) => {
                if let Ok(value) = std::env::var(name) {
                    secrets.push((name.clone(), value));
                }
            }
            Err(e) => {
                let _ = session.release();
                return fail(false, format!("provider key: {e:#}"));
            }
        }
    }
    let trigger = ledger.run(run_id)?;
    // bitterblossom-930: mint once per attempt rather than reusing a stored
    // token across retries -- a fresh capability per attempt is simpler to
    // reason about than invalidating a prior one, and the ledger column is
    // last-write-wins by design (only the current attempt's token is valid).
    let ask_token = uuid::Uuid::new_v4().simple().to_string();
    ledger.set_run_ask_token(run_id, &ask_token)?;
    let plan = WorkspacePlan {
        repos: task.spec.workspace.repos.clone(),
        card: task.card.clone(),
        run_context: serde_json::json!({"run_id": run_id, "task": &task.name, "trigger": {"kind": trigger.trigger_kind, "idempotency_key": trigger.idempotency_key}, "agent": {"name": &task.agent_name, "version": task.agent.version, "role": &task.agent.role, "harness": &task.agent.harness, "model": &task.agent.model}, "substrate": &task.spec.substrate, "roster": &task.roster, "ask_token": &ask_token}).to_string(),
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

    let effective_budget = effective_budget(task);
    let cmd = match harness::build_command(&task.agent, &effective_budget) {
        Ok(c) => c,
        Err(e) => {
            let _ = session.release();
            return fail(false, format!("build_command: {e:#}"));
        }
    };

    ledger.set_attempt_phase(attempt_id, "executing")?;
    ledger.record_progress(run_id, "phase:executing")?;
    let timeout = Duration::from_secs(
        60 * effective_budget
            .timeout_minutes
            .unwrap_or(DEFAULT_TIMEOUT_MINUTES),
    );
    let card_prompt = format!(
        "Read {CARD_FILENAME} in this directory — it is your entire commission. Execute it.\n\n{}",
        task.card
    );
    let mut budget_monitor =
        InFlightBudgetMonitor::new(ledger, run_id, attempt_id, task, &effective_budget);
    let exec = {
        let mut monitor_check =
            |snapshot: &ExecSnapshot<'_>| budget_monitor.observe(snapshot).map(|b| b.reason);
        let poll_interval = in_flight_monitor_interval();
        let mut monitor = substrate::ExecMonitor {
            poll_interval,
            check: &mut monitor_check,
        };
        match session.execute(&cmd, Some(&card_prompt), timeout, Some(&mut monitor)) {
            Ok(r) => r,
            Err(e) => {
                let _ = session.release();
                return fail(true, format!("execute: {e:#}"));
            }
        }
    };
    let observed_stats = budget_monitor.latest_stats();
    let observed_breach = budget_monitor.breach();

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
            stats: AttemptStats::default(),
            cap_breach: None,
        });
    }
    if let Some(reason) = exec.termination_reason {
        let _ = session.release();
        let stats = observed_stats;
        ledger.finish_attempt(
            attempt_id,
            "failure",
            Some(&reason),
            Some(exec.exit_code),
            &stats,
            Some(&artifact_dir),
        )?;
        return Ok(AttemptOutcome::Failure {
            phase_executed: true,
            error: reason,
            stats,
            cap_breach: observed_breach,
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
                stats: AttemptStats::default(),
                cap_breach: None,
            });
        }
        if let Err(error) = ledger.finish_attempt(
            attempt_id,
            "success",
            None,
            Some(exec.exit_code),
            &AttemptStats::default(),
            Some(&artifact_dir),
        ) {
            return Ok(artifact_durability_failure(error, AttemptStats::default()));
        }
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
            stats: AttemptStats::default(),
            cap_breach: None,
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
                stats: AttemptStats::default(),
                cap_breach: None,
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
                    stats: parsed.stats,
                    cap_breach: None,
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
            None,
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
                stats: parsed.stats,
                cap_breach: None,
            });
        }
    }
    session.release().context("release session")?;
    // bitterblossom-930: a parked ask takes precedence over the normal
    // required-artifact contract -- the attempt stopped deliberately, not by
    // finishing its commission, so REPORT.json (or similar) is not expected.
    if attempt_dir.join(ASK_PACKET_FILENAME).exists() {
        if let Err(error) = ledger.finish_attempt(
            attempt_id,
            "parked_on_ask",
            None,
            Some(exec.exit_code),
            &parsed.stats,
            Some(&artifact_dir),
        ) {
            return Ok(artifact_durability_failure(error, parsed.stats));
        }
        ledger.set_attempt_phase(attempt_id, "released")?;
        ledger.record_progress(run_id, "phase:released")?;
        return Ok(AttemptOutcome::Parked {
            stats: parsed.stats,
        });
    }
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
            stats: parsed.stats,
            cap_breach: None,
        });
    }
    if let Err(error) = ledger.finish_attempt(
        attempt_id,
        "success",
        None,
        Some(exec.exit_code),
        &parsed.stats,
        Some(&artifact_dir),
    ) {
        return Ok(artifact_durability_failure(error, parsed.stats));
    }
    ledger.set_attempt_phase(attempt_id, "released")?;
    ledger.record_progress(run_id, "phase:released")?;
    Ok(AttemptOutcome::Success {
        stats: parsed.stats,
    })
}

fn artifact_durability_failure(error: anyhow::Error, stats: AttemptStats) -> AttemptOutcome {
    AttemptOutcome::Failure {
        phase_executed: true,
        error: format!("artifact durability: {error:#}"),
        stats,
        cap_breach: None,
    }
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

fn effective_budget(task: &Task) -> TaskBudget {
    let policy = &task.agent.policy;
    let mut budget = task.spec.budget.clone();
    budget.timeout_minutes = min_opt_u64(budget.timeout_minutes, policy.wall_clock_minutes);
    budget.turn_cap = min_opt_u32(
        min_opt_u32(budget.turn_cap, policy.turn_cap),
        policy.iteration_cap,
    );
    budget.tool_action_cap = min_opt_u32(budget.tool_action_cap, policy.tool_action_cap);
    budget.output_bytes_cap = min_opt_u64(budget.output_bytes_cap, policy.output_bytes_cap);
    budget
}

fn min_opt_u32(a: Option<u32>, b: Option<u32>) -> Option<u32> {
    match (a, b) {
        (Some(a), Some(b)) => Some(a.min(b)),
        (Some(a), None) => Some(a),
        (None, Some(b)) => Some(b),
        (None, None) => None,
    }
}

fn min_opt_u64(a: Option<u64>, b: Option<u64>) -> Option<u64> {
    match (a, b) {
        (Some(a), Some(b)) => Some(a.min(b)),
        (Some(a), None) => Some(a),
        (None, Some(b)) => Some(b),
        (None, None) => None,
    }
}

struct InFlightBudgetMonitor<'a> {
    ledger: &'a Ledger,
    run_id: &'a str,
    attempt_id: i64,
    task_name: &'a str,
    harness: &'a str,
    max_cost_usd: Option<f64>,
    turn_cap: Option<u32>,
    tool_action_cap: Option<u32>,
    output_bytes_cap: Option<u64>,
    policy: String,
    latest_stats: AttemptStats,
    last_recorded_cost: Option<f64>,
    breach: Option<InFlightCapBreach>,
}

struct MonitorDecision {
    reason: String,
}

impl<'a> InFlightBudgetMonitor<'a> {
    fn new(
        ledger: &'a Ledger,
        run_id: &'a str,
        attempt_id: i64,
        task: &'a Task,
        budget: &TaskBudget,
    ) -> Self {
        Self {
            ledger,
            run_id,
            attempt_id,
            task_name: &task.name,
            harness: &task.agent.harness,
            max_cost_usd: budget.max_cost_per_run_usd,
            turn_cap: budget.turn_cap,
            tool_action_cap: budget.tool_action_cap,
            output_bytes_cap: budget.output_bytes_cap,
            policy: task
                .agent
                .policy
                .side_effect_policy
                .clone()
                .unwrap_or_else(|| "kill".to_string()),
            latest_stats: AttemptStats::default(),
            last_recorded_cost: None,
            breach: None,
        }
    }

    fn observe(&mut self, snapshot: &ExecSnapshot<'_>) -> Option<MonitorDecision> {
        if let Some(decision) = self.observe_output_bytes(snapshot) {
            return Some(decision);
        }
        let progress = harness::parse_partial_progress(self.harness, snapshot.stdout);
        if !stats_empty(&progress.stats) {
            self.latest_stats = progress.stats;
            let _ = self
                .ledger
                .update_attempt_stats(self.attempt_id, &self.latest_stats);
        }
        if let Some(turns) = self.latest_stats.turns.and_then(|v| u64::try_from(v).ok()) {
            if let Some(decision) = self.observe_policy_cap("turn_cap", turns, self.turn_cap) {
                return Some(decision);
            }
        }
        if let Some(tool_actions) = progress.tool_actions.and_then(|v| u64::try_from(v).ok()) {
            if let Some(decision) =
                self.observe_policy_cap("tool_action_cap", tool_actions, self.tool_action_cap)
            {
                return Some(decision);
            }
        }
        let cost = self.latest_stats.cost_usd?;
        let changed = match self.last_recorded_cost {
            Some(prior) => (prior - cost).abs() > f64::EPSILON,
            None => true,
        };
        if changed {
            let _ = self.ledger.record_progress(
                self.run_id,
                &format!(
                    "cost observed ${cost:.4} elapsed={}s",
                    snapshot.elapsed.as_secs()
                ),
            );
            self.last_recorded_cost = Some(cost);
        }
        let max = self.max_cost_usd?;
        if cost <= max || self.breach.is_some() {
            return None;
        }
        let reason = format!(
            "in-flight cost cap {}: observed ${cost:.4} > max_cost_per_run_usd ${max:.2}",
            self.policy
        );
        let breach = InFlightCapBreach::Cost {
            observed_cost_usd: cost,
            cap_usd: max,
            policy: self.policy.clone(),
        };
        let _ = self.ledger.record_budget_event(
            Some(self.task_name),
            "in_flight_cap_observed",
            &reason,
        );
        self.breach = Some(breach);
        match self.policy.as_str() {
            "log" => None,
            "kill" | "quarantine" => Some(MonitorDecision { reason }),
            _ => Some(MonitorDecision { reason }),
        }
    }

    fn observe_output_bytes(&mut self, snapshot: &ExecSnapshot<'_>) -> Option<MonitorDecision> {
        let observed = (snapshot.stdout.len() + snapshot.stderr.len()) as u64;
        self.observe_policy_cap("output_bytes_cap", observed, self.output_bytes_cap)
    }

    fn observe_policy_cap(
        &mut self,
        cap_kind: &str,
        observed: u64,
        cap: Option<impl Into<u64> + Copy>,
    ) -> Option<MonitorDecision> {
        let cap = cap.map(Into::into)?;
        if observed <= cap || self.breach.is_some() {
            return None;
        }
        let reason = format!(
            "{cap_kind} {}: observed {observed} > cap {cap}",
            self.policy
        );
        let _ =
            self.ledger
                .record_guard_event("policy_cap_observed", Some(self.task_name), &reason, 1);
        let _ = self.ledger.record_progress(self.run_id, &reason);
        self.breach = Some(InFlightCapBreach::Policy {
            cap_kind: cap_kind.to_string(),
            observed,
            cap,
            policy: self.policy.clone(),
        });
        match self.policy.as_str() {
            "log" => None,
            "kill" | "quarantine" => Some(MonitorDecision { reason }),
            _ => Some(MonitorDecision { reason }),
        }
    }

    fn latest_stats(&self) -> AttemptStats {
        self.latest_stats.clone()
    }

    fn breach(&self) -> Option<InFlightCapBreach> {
        self.breach.clone()
    }
}

fn stats_empty(stats: &AttemptStats) -> bool {
    stats.cost_usd.is_none()
        && stats.tokens_in.is_none()
        && stats.tokens_out.is_none()
        && stats.turns.is_none()
}

fn in_flight_monitor_interval() -> Duration {
    std::env::var("BB_IN_FLIGHT_MONITOR_MS")
        .ok()
        .and_then(|v| v.parse::<u64>().ok())
        .map(Duration::from_millis)
        .filter(|d| !d.is_zero())
        .unwrap_or_else(|| Duration::from_secs(1))
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
