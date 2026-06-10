//! Dispatch: take a pending run through budget check, host lease,
//! prepare, execute, collect, finalize. Mechanical — no judgment about
//! what to fix or whether the agent did a good job.

use std::path::PathBuf;
use std::time::{Duration, Instant};

use anyhow::{Context, Result};

use crate::budget;
use crate::harness;
use crate::ledger::{AttemptStats, Ledger, RunRow};
use crate::spec::{Plane, Task};
use crate::substrate::{self, WorkspacePlan, CARD_FILENAME};

/// Mechanical retries after the initial attempt, for pre-execute failures only.
const MAX_RETRIES: i64 = 2;
const DEFAULT_TIMEOUT_MINUTES: u64 = 60;
const LEASE_WAIT: Duration = Duration::from_secs(60);
const LEASE_POLL: Duration = Duration::from_millis(250);

/// Outcome of one attempt, with the phase it died in.
enum AttemptOutcome {
    Success { stats: AttemptStats },
    Failure { phase_executed: bool, error: String },
}

pub fn dispatch_run(plane: &Plane, ledger: &mut Ledger, run_id: &str) -> Result<RunRow> {
    let run = ledger.run(run_id)?;
    if run.state != "pending" {
        // Replays and racing workers land here; never re-dispatch a run
        // that already left pending.
        return Ok(run);
    }
    let task = plane.task(&run.task)?;

    if let Some(v) = budget::pre_dispatch_check(plane, ledger, task)? {
        ledger.record_budget_event(Some(&task.name), v.kind, &v.detail)?;
        if v.kind == "max_runs_per_day" {
            ledger.park_task(&task.name, &v.detail)?;
        }
        ledger.transition(run_id, "blocked_budget", Some(&v.detail))?;
        crate::notify::notify(
            plane,
            "budget_blocked",
            &serde_json::json!({ "run_id": run_id, "task": task.name, "kind": v.kind, "detail": v.detail }),
        );
        return ledger.run(run_id);
    }

    // Atomic claim: if another worker (or another plane process) already
    // moved this run out of pending, walk away without a second attempt.
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
                // The agent may have had external side effects; "re-run it"
                // is not a recovery semantic. No mechanical retry.
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
    let lease_started = Instant::now();
    loop {
        if ledger.try_acquire_host_lease(&host, run_id)? {
            break;
        }
        if lease_started.elapsed() >= LEASE_WAIT {
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

    let mut secrets = Vec::new();
    for name in &task.agent.secrets {
        match std::env::var(name) {
            Ok(value) => secrets.push((name.clone(), value)),
            Err(_) => {
                let _ = session.release();
                return fail(false, format!("secret env var '{name}' not set"));
            }
        }
    }
    let plan = WorkspacePlan {
        host: task.host(),
        repos: task.spec.workspace.repos.clone(),
        card: task.card.clone(),
        payload: ledger.run_payload(run_id)?,
        pre_command: task.spec.pre_command.clone(),
        post_command: task.spec.post_command.clone(),
        marker: attempt_marker(attempt_id),
        remote_workspace: format!("/home/sprite/bb/{}", task.name),
        checkpoint: task.spec.workspace.checkpoint.clone(),
        secrets,
    };
    if let Err(e) = session.prepare(&plan) {
        let _ = session.release();
        return fail(false, format!("prepare: {e:#}"));
    }
    ledger.set_attempt_phase(attempt_id, "prepared")?;

    let cmd = match harness::build_command(&task.agent, &task.spec.budget) {
        Ok(c) => c,
        Err(e) => {
            let _ = session.release();
            return fail(false, format!("build_command: {e:#}"));
        }
    };

    ledger.set_attempt_phase(attempt_id, "executing")?;
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
            // The process may or may not have started; treat as executed —
            // side effects are possible once we tried to spawn the agent.
            return fail(true, format!("execute: {e:#}"));
        }
    };

    ledger.set_attempt_phase(attempt_id, "collecting")?;
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
            // Unparseable output is a failure with raw output preserved —
            // never a silent zero-cost success.
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

    ledger.set_attempt_phase(attempt_id, "finalizing")?;
    if let Some(post) = &plan.post_command {
        // post_command is finalization (publish artifacts, post replies);
        // a failed finalization must not masquerade as a successful run.
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

    ledger.finish_attempt(
        attempt_id,
        "success",
        None,
        Some(exec.exit_code),
        &parsed.stats,
        Some(&artifact_dir),
    )?;
    ledger.set_attempt_phase(attempt_id, "released")?;
    Ok(AttemptOutcome::Success {
        stats: parsed.stats,
    })
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
