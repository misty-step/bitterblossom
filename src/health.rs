use std::collections::{BTreeMap, HashMap};

use anyhow::Result;
use serde_json::{json, Value};
use time::{format_description::well_known::Rfc3339, OffsetDateTime};

use crate::ledger::{DeadLetterRow, Ledger, RunRow};
use crate::progress;
use crate::spec::Plane;

pub fn status_view(plane: &Plane, ledger: &Ledger) -> Result<Value> {
    let runs = ledger.list_runs(None, None)?;
    let dead_letters = ledger.list_dead_letters()?;
    let leases = ledger.list_host_leases()?;
    let lease_runs = leases
        .iter()
        .filter_map(|lease| {
            ledger
                .run(&lease.run_id)
                .ok()
                .map(|run| (lease.run_id.clone(), run))
        })
        .collect::<HashMap<_, _>>();
    let ingress_events = ledger.latest_ingress_events(200)?;
    let generated_at = OffsetDateTime::now_utc();
    let open_dlq = dead_letters.iter().filter(|d| d.status == "open").count();
    let mut parked_tasks = 0usize;
    let mut tasks = Vec::new();

    for task in plane.tasks.values() {
        let task_runs: Vec<&RunRow> = runs.iter().filter(|r| r.task == task.name).collect();
        let task_dlq: Vec<&DeadLetterRow> = dead_letters
            .iter()
            .filter(|d| d.task == task.name)
            .collect();
        let parked = ledger.parked_reason(&task.name)?;
        if parked.is_some() {
            parked_tasks += 1;
        }
        let by_state = state_counts(&task_runs);
        let latest_open_dlq = task_dlq.iter().copied().find(|d| d.status == "open");
        let latest_failure = task_runs.iter().copied().find(|r| r.state == "failure");
        let latest_recovery = task_runs
            .iter()
            .copied()
            .find(|r| r.state == "awaiting_recovery");
        let pending = task_runs.iter().filter(|r| r.state == "pending").count();
        let running = task_runs.iter().filter(|r| r.state == "running").count();
        let blocked_budget = task_runs
            .iter()
            .filter(|r| r.state == "blocked_budget")
            .count();
        let oldest_pending = task_runs
            .iter()
            .filter(|r| r.state == "pending")
            .map(|r| r.created_at.as_str())
            .min();
        let oldest_pending_age_seconds = oldest_pending
            .and_then(|at| OffsetDateTime::parse(at, &Rfc3339).ok())
            .map(|at| (generated_at - at).whole_seconds().max(0));
        let open_task_dlq = task_dlq.iter().filter(|d| d.status == "open").count();
        let acknowledged_task_dlq = task_dlq
            .iter()
            .filter(|d| d.status == "acknowledged")
            .count();
        let mut running_progress: Vec<Value> = Vec::new();
        for r in task_runs
            .iter()
            .filter(|r| r.state == "running" || r.state == "awaiting_recovery")
        {
            running_progress.push(serde_json::to_value(progress::from_ledger(
                ledger,
                r,
                generated_at,
            )?)?);
        }
        let latest_ingress = ingress_events.iter().find(|e| e.task == task.name);
        let active_lease = leases.iter().find(|l| {
            lease_runs
                .get(&l.run_id)
                .is_some_and(|r| r.task == task.name && r.state == "running")
        });

        tasks.push(json!({
            "task": task.name,
            "agent": format!("{}@v{}", task.agent_name, task.agent.version),
            "harness": task.agent.harness,
            "model": task.agent.model,
            "substrate": task.spec.substrate,
            "verdict": task.spec.verdict,
            "parked": parked,
            "budget": {
                "runs_today": ledger.runs_today(&task.name)?,
                "max_runs_per_day": task.spec.budget.max_runs_per_day,
                "max_cost_per_run_usd": task.spec.budget.max_cost_per_run_usd,
                "timeout_minutes": task.spec.budget.timeout_minutes,
            },
            "runs": {
                "recent": task_runs.len(),
                "by_state": by_state,
                "latest": task_runs.first().map(|r| run_summary(r)),
                "latest_failure": latest_failure.map(run_summary),
                "cost_usd": task_runs.iter().filter_map(|r| r.cost_usd).sum::<f64>(),
                "duration_ms": task_runs.iter().filter_map(|r| r.duration_ms).sum::<i64>(),
            },
            "queue": {
                "pending": pending,
                "running": running,
                "blocked_budget": blocked_budget,
                "oldest_pending_created_at": oldest_pending,
                "oldest_pending_age_seconds": oldest_pending_age_seconds,
            },
            "dlq": {
                "open": open_task_dlq,
                "acknowledged": acknowledged_task_dlq,
                "total": task_dlq.len(),
                "latest_open": latest_open_dlq.map(dlq_summary),
            },
            "progress": {
                "running": running_progress,
            },
            "ingress": {
                "events": ledger.ingress_event_count(&task.name)?,
                "latest": latest_ingress,
            },
            "lease": active_lease,
            "safe_next_actions": safe_actions(
                &task.name,
                parked.as_deref(),
                latest_open_dlq,
                latest_recovery,
                latest_failure,
                pending,
                generated_at,
            ),
        }));
    }
    let paused = ledger.plane_paused()?;
    let guard_counts = ledger.guard_event_counts()?;
    let recent_guards = ledger.list_guard_events(50)?;
    let notification_counts = ledger.notification_outbox_counts()?;
    let recent_notifications = ledger.list_notification_outbox(50)?;
    let running: Vec<&RunRow> = runs.iter().filter(|r| r.state == "running").collect();
    let in_flight_cost = ledger.in_flight_cost()?;
    // Conservative reservation: the worst-case cost each in-flight run could
    // still incur, bounded by its task's per-run cap. The daily ceiling is
    // enforced separately on every dispatch (budget::pre_dispatch_check).
    let reserved_usd: f64 = running
        .iter()
        .filter_map(|r| plane.task(&r.task).ok())
        .filter_map(|t| t.spec.budget.max_cost_per_run_usd)
        .sum();

    Ok(json!({
        "generated_at": generated_at.format(&Rfc3339)?,
        "summary": {
            "tasks": plane.tasks.len(),
            "parked_tasks": parked_tasks,
            "open_dlq": open_dlq,
            "active_leases": leases.len(),
            "recent_ingress_events": ingress_events.len(),
            "cost_today_usd": ledger.cost_today()?,
            "max_cost_per_day_usd": plane.spec.budget.max_cost_per_day_usd,
            "plane_paused": paused.is_some(),
        },
        "guards": {
            "plane_paused": paused.is_some(),
            "paused_reason": paused.as_ref().map(|(r, _)| r.clone()),
            "paused_at": paused.as_ref().map(|(_, a)| a.clone()),
            "ingress": {
                "max_body_bytes": plane.spec.ingress.max_body_bytes,
                "oversized_rejections": guard_total(&guard_counts, "ingress_oversized"),
            },
            "cron": {
                "max_catchup_fires": plane.spec.ingress.max_cron_catchup_fires,
                "skipped_catchup_fires": guard_total(&guard_counts, "cron_collapse"),
            },
            "notify": {
                "failed": guard_total(&guard_counts, "notify_failed"),
                "outbox": {
                    "pending": outbox_total(&notification_counts, "pending"),
                    "failed": outbox_total(&notification_counts, "failed"),
                    "delivered": outbox_total(&notification_counts, "delivered"),
                    "acknowledged": outbox_total(&notification_counts, "acknowledged"),
                },
                "recent_outbox": recent_notifications,
            },
            "in_flight": {
                "runs": running.len(),
                "cost_usd": in_flight_cost,
                "reserved_usd": reserved_usd,
                "policy": "reserved = sum(max_cost_per_run_usd) over running runs; the global daily ceiling (max_cost_per_day_usd) is still enforced by budget::pre_dispatch_check on every dispatch.",
            },
            "guard_event_counts": guard_counts,
            "recent_guard_events": recent_guards,
        },
        "leases": leases,
        "ingress": {
            "recent": ingress_events,
        },
        "freshness_contracts": progress::freshness_contracts(),
        "tasks": tasks,
    }))
}

fn state_counts(runs: &[&RunRow]) -> BTreeMap<String, usize> {
    let mut out = BTreeMap::new();
    for r in runs {
        *out.entry(r.state.clone()).or_default() += 1;
    }
    out
}
fn guard_total(counts: &[crate::ledger::GuardEventCount], kind: &str) -> i64 {
    counts
        .iter()
        .find(|c| c.kind == kind)
        .map(|c| c.total)
        .unwrap_or(0)
}

fn outbox_total(counts: &[crate::ledger::NotificationOutboxCount], status: &str) -> i64 {
    counts
        .iter()
        .find(|c| c.status == status)
        .map(|c| c.total)
        .unwrap_or(0)
}

fn run_summary(r: &RunRow) -> Value {
    json!({
        "id": r.id,
        "state": r.state,
        "reason": r.state_reason,
        "agent": r.agent_name.as_ref().zip(r.agent_version).map(|(n, v)| format!("{n}@v{v}")),
        "cost_usd": r.cost_usd,
        "duration_ms": r.duration_ms,
        "created_at": r.created_at,
        "updated_at": r.updated_at,
    })
}

fn dlq_summary(d: &DeadLetterRow) -> Value {
    json!({
        "id": d.id,
        "run_id": d.run_id,
        "status": d.status,
        "error": d.error,
        "created_at": d.created_at,
    })
}

fn safe_actions(
    task: &str,
    parked: Option<&str>,
    dlq: Option<&DeadLetterRow>,
    recovery: Option<&RunRow>,
    failure: Option<&RunRow>,
    pending: usize,
    generated_at: OffsetDateTime,
) -> Vec<Value> {
    let mut out = Vec::new();
    if let Some(reason) = parked {
        out.push(json!({
            "kind": "unpark_after_reason_cleared",
            "reason": reason,
            "command": format!("bb task unpark {task}"),
        }));
    }
    if let Some(d) = dlq {
        out.push(json!({
            "kind": "replay_pre_execute_dlq",
            "reason": d.error,
            "command": format!("bb dlq replay {}", d.id),
        }));
    }
    if let Some(r) = recovery {
        let age_seconds = OffsetDateTime::parse(&r.updated_at, &Rfc3339)
            .map(|at| (generated_at - at).whole_seconds().max(0))
            .unwrap_or(0);
        let stale = age_seconds >= progress::RECOVERY_STALE_SECONDS;
        out.push(json!({
            "kind": if stale { "escalate_stale_recovery" } else { "resolve_after_side_effect_inspection" },
            "reason": r.state_reason,
            "age_seconds": age_seconds,
            "stale_after_seconds": progress::RECOVERY_STALE_SECONDS,
            "command": format!("bb runs resolve {} success|failure", r.id),
        }));
    }
    if let Some(r) = failure {
        out.push(json!({
            "kind": "inspect_artifact",
            "reason": r.state_reason,
            "command": format!("bb runs show {} --json", r.id),
        }));
    }
    if pending > 0 {
        out.push(json!({
            "kind": "wait_or_cancel_pending",
            "reason": format!("{pending} pending run(s)"),
            "command": "bb runs list --state pending --json",
        }));
    }
    if out.is_empty() {
        out.push(json!({
            "kind": "monitor",
            "reason": "no operator action suggested",
            "command": "bb status --json",
        }));
    }
    out
}
