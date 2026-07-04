use anyhow::Result;

use crate::ledger::Ledger;
use crate::spec::{Plane, Task};
#[derive(Clone, Debug)]
pub struct Violation {
    pub kind: &'static str,
    pub detail: String,
}

#[derive(Clone, Debug)]
pub enum DispatchAdmission {
    Running,
    Blocked(Violation),
    NotPending,
}

pub fn admit_dispatch(
    plane: &Plane,
    ledger: &mut Ledger,
    task: &Task,
    run_id: &str,
) -> Result<DispatchAdmission> {
    ledger.conn.execute_batch("BEGIN IMMEDIATE")?;
    let result = (|| {
        if ledger.run_state(run_id)? != "pending" {
            return Ok(DispatchAdmission::NotPending);
        }
        if let Some(v) = pre_dispatch_check(plane, ledger, task)? {
            ledger.record_budget_event(Some(&task.name), v.kind, &v.detail)?;
            if v.kind == "max_runs_per_day" {
                ledger.park_task(&task.name, &v.detail)?;
            }
            ledger.transition(run_id, "blocked_budget", Some(&v.detail))?;
            return Ok(DispatchAdmission::Blocked(v));
        }
        if ledger.try_transition(run_id, "running", None)? {
            Ok(DispatchAdmission::Running)
        } else {
            Ok(DispatchAdmission::NotPending)
        }
    })();
    match result {
        Ok(admission) => {
            ledger.conn.execute_batch("COMMIT")?;
            Ok(admission)
        }
        Err(err) => {
            let _ = ledger.conn.execute_batch("ROLLBACK");
            Err(err)
        }
    }
}

pub fn pre_dispatch_check(
    plane: &Plane,
    ledger: &Ledger,
    task: &Task,
) -> Result<Option<Violation>> {
    if let Some(reason) = ledger.parked_reason(&task.name)? {
        return Ok(Some(Violation {
            kind: "task_parked",
            detail: format!("task parked: {reason}"),
        }));
    }
    budget_limits(plane, ledger, task)
}

/// Spend/quota limits that survive an unpark — a released run would still
/// re-block on these. The task park is handled by `pre_dispatch_check`.
pub fn budget_limits(plane: &Plane, ledger: &Ledger, task: &Task) -> Result<Option<Violation>> {
    if let Some(max) = task.spec.budget.max_runs_per_day {
        let today = ledger.runs_today(&task.name)?;
        if today >= max as i64 {
            return Ok(Some(Violation {
                kind: "max_runs_per_day",
                detail: format!("{today} runs today >= max_runs_per_day {max}"),
            }));
        }
    }
    if let Some(ceiling) = plane.spec.budget.max_cost_per_day_usd {
        let spent = ledger.cost_today()?;
        if spent >= ceiling {
            return Ok(Some(Violation {
                kind: "global_daily_ceiling",
                detail: format!("${spent:.2} spent today >= ceiling ${ceiling:.2}"),
            }));
        }
    }
    Ok(None)
}
pub fn post_run_check(task: &Task, cost_usd: Option<f64>) -> Option<Violation> {
    let (Some(max), Some(cost)) = (task.spec.budget.max_cost_per_run_usd, cost_usd) else {
        return None;
    };
    if cost > max {
        return Some(Violation {
            kind: "max_cost_per_run",
            detail: format!("run cost ${cost:.4} > max_cost_per_run_usd ${max:.2}"),
        });
    }
    None
}
