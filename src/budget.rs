use anyhow::Result;

use crate::ledger::Ledger;
use crate::spec::{Plane, Task};
use crate::workflow::WorkflowDoc;
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

/// Refusal shared by standard-task and workflow admission. A declared
/// metered parent key is not safe on a harness with no eventual dollar report
/// unless an effective capped child key is declared for the exact provider.
pub fn metered_parent_key_violation(
    agent_name: &str,
    harness: &str,
    provider: &str,
    secrets: &[String],
    optional_secrets: &[String],
    provider_key_name: Option<&str>,
    provider_spend_cap_usd: Option<f64>,
) -> Option<Violation> {
    let holds_metered_key = secrets
        .iter()
        .chain(optional_secrets.iter())
        .any(|s| s == crate::provider_keys::OPENROUTER_SECRET_ENV);
    let child_key_cap_effective =
        provider == "openrouter" && provider_key_name.is_some() && provider_spend_cap_usd.is_some();
    if holds_metered_key && !crate::harness::reports_cost(harness) && !child_key_cap_effective {
        return Some(Violation {
            kind: "cost_blind_harness",
            detail: format!(
                "agent '{agent_name}' holds {secret} on harness '{harness}', which cannot report cost_usd — every plane spend control is blind to this run; declare policy.provider_key_name + policy.provider_spend_cap_usd on provider openrouter and mint the capped child key, or move the workload to a cost-reporting harness",
                secret = crate::provider_keys::OPENROUTER_SECRET_ENV,
            ),
        });
    }
    None
}

/// Apply one serialized admission projection to a workflow. The caller owns
/// the surrounding BEGIN IMMEDIATE, so standard task spend, all workflow
/// realized spend, active reservations, and this run's pinned reservation are
/// observed and admitted as one atomic decision.
pub fn workflow_admission_limit(
    plane: &Plane,
    ledger: &Ledger,
    workflow_name: &str,
    doc: &WorkflowDoc,
    additional_reservation: f64,
) -> Result<Option<Violation>> {
    for step in &doc.steps {
        let provider = step.agent.provider.as_deref().unwrap_or("openrouter");
        if let Some(v) = metered_parent_key_violation(
            &step.agent.name,
            &step.agent.harness,
            provider,
            &step.agent.secrets,
            &[],
            None,
            None,
        ) {
            return Ok(Some(v));
        }
    }

    let standard_observed = ledger.standard_cost_today()?.0;
    let workflow_spend = ledger.workflow_spend_today_all()?;
    let plane_projected = standard_observed
        + workflow_spend.observed_usd
        + workflow_spend.estimated_usd
        + workflow_spend.reserved_usd
        + additional_reservation;
    if let Some(ceiling) = plane.spec.budget.max_cost_per_day_usd {
        if plane_projected > ceiling {
            return Ok(Some(Violation {
                kind: "global_daily_ceiling",
                detail: format!(
                    "plane daily ceiling: projected ${plane_projected:.4} (standard observed ${standard_observed:.4} + workflow observed ${:.4} + estimated ${:.4} + reserved ${:.4} + new reservation ${additional_reservation:.4}) > max_cost_per_day_usd ${ceiling:.2}",
                    workflow_spend.observed_usd,
                    workflow_spend.estimated_usd,
                    workflow_spend.reserved_usd,
                ),
            }));
        }
    }

    let wf = ledger.workflow_by_name(workflow_name)?;
    let own = ledger.workflow_spend_today_by_id(&wf.id)?;
    if let Some(ceiling) = doc.policies.max_cost_per_day_usd {
        let projected =
            own.observed_usd + own.estimated_usd + own.reserved_usd + additional_reservation;
        if projected > ceiling {
            return Ok(Some(Violation {
                kind: "workflow_daily_ceiling",
                detail: format!(
                    "workflow daily ceiling: projected ${projected:.4} (observed ${:.4} + estimated ${:.4} + reserved ${:.4} + new reservation ${additional_reservation:.4}) > max_cost_per_day_usd ${ceiling:.2}",
                    own.observed_usd,
                    own.estimated_usd,
                    own.reserved_usd,
                ),
            }));
        }
    }
    Ok(None)
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
    if let Some(v) = metered_parent_key_violation(
        &task.agent_name,
        &task.agent.harness,
        task.agent.provider(),
        &task.agent.secrets,
        &task.agent.optional_secrets,
        task.agent.policy.provider_key_name.as_deref(),
        task.agent.policy.provider_spend_cap_usd,
    ) {
        return Ok(Some(v));
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
        let standard_spent = ledger.standard_cost_today()?.0;
        let workflow_spend = ledger.workflow_spend_today_all()?;
        let projected = standard_spent
            + workflow_spend.observed_usd
            + workflow_spend.estimated_usd
            + workflow_spend.reserved_usd;
        if projected >= ceiling {
            return Ok(Some(Violation {
                kind: "global_daily_ceiling",
                detail: format!(
                    "${projected:.2} spent or reserved today >= ceiling ${ceiling:.2} (standard ${standard_spent:.2} + workflow observed ${:.2} + estimated ${:.2} + reserved ${:.2})",
                    workflow_spend.observed_usd,
                    workflow_spend.estimated_usd,
                    workflow_spend.reserved_usd,
                ),
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
