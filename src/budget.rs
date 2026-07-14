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
    // bitterblossom-969: every cost control on this plane — the per-run cap,
    // the global and per-repo daily ceilings, the in-flight overrun monitor —
    // reads parsed attempt cost_usd. An agent holding the metered
    // OPENROUTER_API_KEY on a harness that cannot report cost is invisible to
    // all of them; the only control that can survive is a provider-side
    // child-key spend cap. The declared secret NAME is the definitive signal:
    // the auth label and the free-form provider string deliberately play no
    // part in the refusal — both were executed bypasses in the PR #1005
    // review (auth = "subscription" and provider = "openrouter " each let the
    // parent key flow uncapped with cost NULL). The child-key exemption
    // requires the cap to be *effective*: bb mints child keys for provider
    // "openrouter" exactly, so a declared cap on any other provider string is
    // a dead letter and does not admit. Prepare swaps the capped child key in
    // and refuses to run if it was never minted.
    let holds_metered_key = task
        .agent
        .secrets
        .iter()
        .chain(task.agent.optional_secrets.iter())
        .any(|s| s == crate::provider_keys::OPENROUTER_SECRET_ENV);
    let child_key_cap_effective = task.agent.provider() == "openrouter"
        && task.agent.policy.provider_key_name.is_some()
        && task.agent.policy.provider_spend_cap_usd.is_some();
    if holds_metered_key
        && !crate::harness::reports_cost(&task.agent.harness)
        && !child_key_cap_effective
    {
        return Ok(Some(Violation {
            kind: "cost_blind_harness",
            detail: format!(
                "agent '{agent}' holds {secret} on harness '{harness}', which cannot \
                 report cost_usd — every plane spend control is blind to this run; \
                 declare policy.provider_key_name + policy.provider_spend_cap_usd on \
                 provider \"openrouter\" and mint the capped child key \
                 (`bb keys mint {agent}`), or move the workload to a cost-reporting \
                 harness",
                agent = task.agent_name,
                harness = task.agent.harness,
                secret = crate::provider_keys::OPENROUTER_SECRET_ENV,
            ),
        }));
    }
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
    if let Some((repo_prefix, ceiling)) = repo_daily_ceiling(plane, task) {
        let spent = ledger.cost_today_for_repo(repo_prefix)?;
        if spent >= ceiling {
            return Ok(Some(Violation {
                kind: "repo_daily_ceiling",
                detail: format!(
                    "${spent:.2} spent today by repo '{repo_prefix}' >= ceiling ${ceiling:.2}"
                ),
            }));
        }
    }
    Ok(None)
}

/// Cost governor slice 1 (bitterblossom-960): a repo-owned task's name is
/// always `<repo.name>/<short>` (`load_workload_repo_tasks` in spec.rs is
/// the single source of that convention), so the repo's own declared
/// namespace is recoverable from the task name alone -- no extra field on
/// `Task` needed. Plane-owned (non-repo) tasks never match a workload repo
/// name and fall through to `None`, unaffected by this ceiling.
fn repo_daily_ceiling<'a>(plane: &'a Plane, task: &Task) -> Option<(&'a str, f64)> {
    let (prefix, _) = task.name.split_once('/')?;
    let repo = plane
        .spec
        .workload_repos
        .iter()
        .find(|r| r.name == prefix)?;
    Some((repo.name.as_str(), repo.max_cost_per_day_usd?))
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
