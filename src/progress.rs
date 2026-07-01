//! Progress/stale classification for long-running attempts (backlog 087).
//!
//! Mechanism only. Reads ledger evidence — the most recent `progress` event, the
//! `boot_probe` event recorded by recovery, and the latest attempt phase — and
//! produces a machine-readable classification with an explicit operator
//! `safe_next_action`. It never decides to kill or retry executing work that
//! may have external side effects; stale executing work points the operator at a
//! probe (`bb recover --json`) instead.
//!
//! The progress markers are written by the dispatcher at attempt phase
//! transitions and by the foreground heartbeat thread; both reuse
//! [`crate::ledger::Ledger::record_progress`]. The stale threshold is a single
//! global default here — per-task configurability is a follow-up promotion
//! metric, not part of this mechanism slice.

use anyhow::Result;
use serde::Serialize;
use time::{format_description::well_known::Rfc3339, OffsetDateTime};

use crate::ledger::{phase_reached, Ledger, RunRow};

/// Default progress-stale threshold. A running attempt whose most recent
/// progress marker is older than this is classified `stale_executing` so an
/// operator can probe before acting. Deliberately shorter than a typical run
/// timeout: the goal is visibility ("is this making progress?"), not a kill
/// switch.
pub const PROGRESS_STALE_SECONDS: i64 = 1800;

#[derive(Serialize)]
pub struct SafeNextAction {
    pub kind: &'static str,
    pub reason: String,
    pub command: Option<String>,
}

#[derive(Serialize)]
pub struct ProgressView {
    pub run_id: String,
    pub classification: &'static str,
    pub last_progress_at: Option<String>,
    pub age_seconds: Option<i64>,
    pub threshold_seconds: i64,
    pub probe: Option<String>,
    pub attempt_phase: Option<String>,
    pub safe_next_action: SafeNextAction,
}

/// Build the progress view for `run` by querying the ledger for its latest
/// progress marker, probe, and attempt phase.
pub fn from_ledger(ledger: &Ledger, run: &RunRow, now: OffsetDateTime) -> Result<ProgressView> {
    let last_progress_at = ledger.last_progress_at(&run.id)?;
    let probe = ledger.latest_probe(&run.id)?;
    let attempt_phase = ledger.attempts(&run.id)?.last().map(|a| a.phase.clone());
    Ok(classify(
        run,
        attempt_phase.as_deref(),
        last_progress_at.as_deref(),
        probe.as_deref(),
        now,
    ))
}

/// Pure classifier over the evidence a run carries. Kept free of I/O so the
/// fixture states in the test suite are exact and reproducible. Each arm
/// decides only the `classification` and its `safe_next_action`; the shared
/// envelope fields are filled once at the end.
pub fn classify(
    run: &RunRow,
    attempt_phase: Option<&str>,
    last_progress_at: Option<&str>,
    probe: Option<&str>,
    now: OffsetDateTime,
) -> ProgressView {
    let threshold = PROGRESS_STALE_SECONDS;
    let age_seconds = last_progress_at.and_then(|at| {
        OffsetDateTime::parse(at, &Rfc3339)
            .ok()
            .map(|t| (now - t).whole_seconds().max(0))
    });
    let cmd = |c: String| Some(c);
    let resolve = || cmd(format!("bb runs resolve {} success|failure", run.id));
    let show = || cmd(format!("bb runs show {} --json", run.id));

    // With no progress markers at all, fall back to time since run creation.
    let since_start = OffsetDateTime::parse(&run.created_at, &Rfc3339)
        .map(|t| (now - t).whole_seconds().max(0))
        .unwrap_or(threshold);

    let (classification, safe_next_action) = match run.state.as_str() {
        "success" | "failure" => (
            "done",
            SafeNextAction {
                kind: "none",
                reason: format!("run is terminal: {}", run.state),
                command: None,
            },
        ),
        "pending" | "blocked_budget" => (
            "queued",
            SafeNextAction {
                kind: "wait_or_cancel_pending",
                reason: format!("run is {}", run.state),
                command: cmd("bb runs list --state pending --json".into()),
            },
        ),
        "running" => {
            // A run that just started is fresh; one that has sat running past
            // the threshold without recording any marker is stale.
            let stale = match age_seconds {
                Some(age) => age >= threshold,
                None => since_start >= threshold,
            };
            let elapsed = age_seconds.unwrap_or(since_start);
            if stale {
                (
                    "stale_executing",
                    SafeNextAction {
                        kind: "probe_before_action",
                        reason: format!(
                            "no progress for {elapsed}s; probe before action — \
                             do not auto-kill (side effects possible)"
                        ),
                        command: cmd("bb recover --json".into()),
                    },
                )
            } else {
                (
                    "fresh",
                    SafeNextAction {
                        kind: "monitor",
                        reason: format!("progress within threshold ({elapsed}s)"),
                        command: cmd("bb status --json".into()),
                    },
                )
            }
        }
        "awaiting_recovery" => {
            let executed = attempt_phase
                .map(|p| phase_reached(p, "executing"))
                .unwrap_or(false);
            match probe {
                Some("dead") if executed => (
                    "dead_executing",
                    SafeNextAction {
                        kind: "resolve_after_side_effect_inspection",
                        reason: "probe dead at or after executing; side effects may exist".into(),
                        command: resolve(),
                    },
                ),
                Some("dead") => (
                    "dead_pre_attempt",
                    SafeNextAction {
                        kind: "replay_or_resolve",
                        reason: "probe dead before executing; replay dead letter or resolve".into(),
                        command: resolve(),
                    },
                ),
                Some(p) if p.starts_with("unknown") => (
                    "unknown_probe",
                    SafeNextAction {
                        kind: "inspect_manually",
                        reason: format!("probe inconclusive ({p}); operator must inspect"),
                        command: show(),
                    },
                ),
                _ => (
                    "unknown_probe",
                    SafeNextAction {
                        kind: "inspect_manually",
                        reason: "no probe recorded; operator must inspect".into(),
                        command: show(),
                    },
                ),
            }
        }
        _ => (
            "unknown",
            SafeNextAction {
                kind: "inspect_manually",
                reason: format!("unrecognized run state: {}", run.state),
                command: show(),
            },
        ),
    };

    ProgressView {
        run_id: run.id.clone(),
        classification,
        last_progress_at: last_progress_at.map(str::to_owned),
        age_seconds,
        threshold_seconds: threshold,
        probe: probe.map(str::to_owned),
        attempt_phase: attempt_phase.map(str::to_owned),
        safe_next_action,
    }
}
