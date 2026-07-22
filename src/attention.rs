use anyhow::Result;
use serde::Serialize;
use time::OffsetDateTime;

use crate::ledger::Ledger;
use crate::progress;
use crate::spec::Plane;

#[derive(Clone, Debug, Serialize)]
pub struct AttentionDebt {
    pub open_dlq: usize,
    pub parked_tasks: usize,
    pub stale_runs: usize,
    pub awaiting_recovery: usize,
    pub notification_pending: i64,
    pub notification_failed: i64,
    pub blocking: bool,
    pub reason: String,
}

pub fn scan(plane: &Plane, ledger: &Ledger, now: OffsetDateTime) -> Result<AttentionDebt> {
    let open_dlq = usize::try_from(ledger.open_dead_letter_count(None)?)
        .map_err(|_| anyhow::anyhow!("open dead-letter count exceeds usize"))?;
    let runs = ledger.list_runs(None, None)?;
    let notification_counts = ledger.notification_outbox_counts()?;
    let mut parked_tasks = 0usize;
    for task in plane.tasks.values() {
        if ledger.parked_reason(&task.name)?.is_some() {
            parked_tasks += 1;
        }
    }
    let mut stale_runs = 0usize;
    let mut awaiting_recovery = 0usize;
    for run in runs
        .iter()
        .filter(|r| r.state == "running" || r.state == "awaiting_recovery")
    {
        if run.state == "awaiting_recovery" {
            awaiting_recovery += 1;
        }
        let view = progress::from_ledger(ledger, run, now)?;
        if view.classification == "stale_executing" {
            stale_runs += 1;
        }
    }
    let notification_pending = outbox_total(&notification_counts, "pending");
    let notification_failed = outbox_total(&notification_counts, "failed");
    Ok(build_debt(
        open_dlq,
        parked_tasks,
        stale_runs,
        awaiting_recovery,
        notification_pending,
        notification_failed,
    ))
}

pub fn scan_task(ledger: &Ledger, task: &str, now: OffsetDateTime) -> Result<AttentionDebt> {
    let open_dlq = usize::try_from(ledger.open_dead_letter_count(Some(task))?)
        .map_err(|_| anyhow::anyhow!("open dead-letter count exceeds usize"))?;
    let runs = ledger.list_runs(Some(task), None)?;
    let parked_tasks = usize::from(ledger.parked_reason(task)?.is_some());
    let mut stale_runs = 0usize;
    let mut awaiting_recovery = 0usize;
    for run in runs
        .iter()
        .filter(|r| r.state == "running" || r.state == "awaiting_recovery")
    {
        if run.state == "awaiting_recovery" {
            awaiting_recovery += 1;
        }
        let view = progress::from_ledger(ledger, run, now)?;
        if view.classification == "stale_executing" {
            stale_runs += 1;
        }
    }
    Ok(build_debt(
        open_dlq,
        parked_tasks,
        stale_runs,
        awaiting_recovery,
        0,
        0,
    ))
}

fn build_debt(
    open_dlq: usize,
    parked_tasks: usize,
    stale_runs: usize,
    awaiting_recovery: usize,
    notification_pending: i64,
    notification_failed: i64,
) -> AttentionDebt {
    let mut reasons = Vec::new();
    if open_dlq > 0 {
        reasons.push(format!("open_dlq={open_dlq}"));
    }
    if parked_tasks > 0 {
        reasons.push(format!("parked_tasks={parked_tasks}"));
    }
    if stale_runs > 0 {
        reasons.push(format!("stale_runs={stale_runs}"));
    }
    if awaiting_recovery > 0 {
        reasons.push(format!("awaiting_recovery={awaiting_recovery}"));
    }
    if notification_pending > 0 {
        reasons.push(format!("notification_pending={notification_pending}"));
    }
    if notification_failed > 0 {
        reasons.push(format!("notification_failed={notification_failed}"));
    }
    let blocking = !reasons.is_empty();
    AttentionDebt {
        open_dlq,
        parked_tasks,
        stale_runs,
        awaiting_recovery,
        notification_pending,
        notification_failed,
        blocking,
        reason: if blocking {
            reasons.join(" ")
        } else {
            "clear".into()
        },
    }
}

fn outbox_total(counts: &[crate::ledger::NotificationOutboxCount], status: &str) -> i64 {
    counts
        .iter()
        .find(|c| c.status == status)
        .map(|c| c.total)
        .unwrap_or(0)
}
