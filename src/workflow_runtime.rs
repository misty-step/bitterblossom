//! Workflow runtime (bitterblossom-workflow-runtime-v1): trigger -> step
//! (pinned agent + natural-language goal) -> result -> route.
//!
//! Mechanism only. The plane owns event acceptance and dedupe, step
//! sequencing over declared routes, the completion-outcome contract, cycle
//! guards, child-agent evidence with monotonic authority narrowing, and
//! run-group state. What a goal means, what an outcome name means, and what
//! agents do stays outside the spine: the outcome vocabulary is workflow
//! config, and no route decision ever comes from matching prose.
//!
//! Acceptance: every trigger source — external webhook, schedule, internal
//! (another workflow/agent), and synthetic test — builds one
//! [`TriggerEnvelope`] and passes it to [`accept`]. There is exactly one
//! acceptance path, one dedupe rule, and one disposition vocabulary.
//!
//! Execution: an accepted run is one run group. Steps execute sequentially
//! through the substrate/harness seams dispatch already uses. A step with
//! zero or one route completes on successful harness completion (no result
//! schema); a step with two or more routes must supply exactly one declared
//! outcome through the completion tool (`OUTCOME.json`) or the run is
//! `incomplete` — never guessed. Declared child agents are recorded as
//! evidence under their parent step and never become catalog entries.
//! Guards (external stop signal, rounds, elapsed, spend) are checked before
//! every attempt; the first fired guard stops the run with its name in the
//! record. Once execution has begun nothing is blindly retried: failures
//! and inherited in-flight runs become explicit operator states.

use std::collections::HashMap;
use std::path::PathBuf;
use std::time::{Duration, Instant};

use anyhow::{bail, Context, Result};
use chrono::{DateTime, Utc};
use rusqlite::{params, OptionalExtension};
use serde::{Deserialize, Serialize};

use crate::auth::LiveLease;
use crate::budget;
use crate::harness;
use crate::ledger::{new_id, now, AttemptStats, Ledger};
use crate::spec::{AgentSpec, AuthClass, Plane, TaskBudget};
use crate::substrate::{self, ExecMonitor, ExecSnapshot, WorkspacePlan};
use crate::workflow::{
    AcceptOutcome, LaunchSnapshot, StepAgent, WorkflowAction, WorkflowDoc, WorkflowPolicies,
    WorkflowStep, ROUTE_DONE,
};

/// The completion tool: a branching step's agent writes this file with one
/// declared outcome. Single-route steps need no result schema.
pub const OUTCOME_FILENAME: &str = "OUTCOME.json";
/// Dynamic child-agent evidence a step's agent declares for the run tree.
pub const CHILD_AGENTS_FILENAME: &str = "CHILD_AGENTS.json";
const DEFAULT_STEP_TIMEOUT_MINUTES: u64 = 30;
/// A queued runner must not hold a worker thread behind one host forever.
const WORKFLOW_LEASE_WAIT: Duration = Duration::from_secs(60);
/// Absolute per-run-group step-attempt ceiling (defense-in-depth behind the
/// declared cycle guards; see `execute_claimed`).
const MAX_RUN_GROUP_ATTEMPTS: i64 = 256;
/// Size cap for agent-controlled contract files (`OUTCOME.json`,
/// `CHILD_AGENTS.json`), matching the ingress body default. The substrate
/// release path enforces the same limit on collected artifacts; this
/// read-side check is defense-in-depth for layouts where the executor reads
/// files release never collected.
const MAX_CONTRACT_FILE_BYTES: u64 = 1_048_576;

// --- one normalized acceptance contract ------------------------------------

/// Where a trigger event came from. Every source normalizes to this enum;
/// nothing downstream ever branches on transport details.
#[derive(Debug, Clone, PartialEq, Eq)]
pub enum TriggerSource {
    /// External connector event (webhook delivery on a route).
    External,
    /// Clock/schedule fire.
    Schedule,
    /// Event emitted by another workflow or agent.
    Internal,
    /// Synthetic test/fixture event.
    Test,
    /// Operator-initiated.
    Manual,
    /// Deliberate replay of a prior event.
    Replay,
}

impl TriggerSource {
    pub fn kind(&self) -> &'static str {
        match self {
            TriggerSource::External => "webhook",
            TriggerSource::Schedule => "cron",
            TriggerSource::Internal => "internal",
            TriggerSource::Test => "test",
            TriggerSource::Manual => "manual",
            TriggerSource::Replay => "replay",
        }
    }

    pub fn from_kind(kind: &str) -> Result<Self> {
        Ok(match kind {
            "webhook" => TriggerSource::External,
            "cron" => TriggerSource::Schedule,
            "internal" => TriggerSource::Internal,
            "test" => TriggerSource::Test,
            "manual" => TriggerSource::Manual,
            "replay" => TriggerSource::Replay,
            other => bail!(
                "trigger kind '{other}' is unknown (known: {})",
                crate::workflow::TRIGGER_KINDS.join(", ")
            ),
        })
    }
}

/// The single normalized acceptance contract every trigger source shares.
#[derive(Debug)]
pub struct TriggerEnvelope {
    pub workflow: String,
    pub source: TriggerSource,
    /// JSON payload, validated at acceptance.
    pub payload: Option<String>,
    /// Idempotency handle: a repeat acceptance with the same key returns the
    /// original run as a duplicate instead of creating a second one.
    pub dedupe_key: Option<String>,
}

/// THE acceptance path. Webhook ingress, the cron scheduler, internal
/// emitters, synthetic tests, the CLI, and the HTTP API all build a
/// [`TriggerEnvelope`] and call this one function.
pub fn accept(plane: &Plane, ledger: &Ledger, envelope: &TriggerEnvelope) -> Result<AcceptOutcome> {
    crate::workflow_service::WorkflowService::new(
        plane,
        ledger,
        crate::workflow_service::auth_context_for_controller(),
    )
    .accept(envelope)
}

/// Find the active workflow (if any) declaring a webhook trigger on `route`.
/// Reads the ACTIVE revision document, so pausing or revising a workflow
/// immediately changes what the route accepts.
pub fn webhook_workflow_target(
    ledger: &Ledger,
    route: &str,
) -> Result<Option<(String, crate::workflow::WorkflowTrigger)>> {
    let route = crate::ingress::normalize_route(route);
    let mut active = None;
    let mut paused = None;
    for wf in ledger.list_workflows()? {
        if !matches!(wf.state.as_str(), "active" | "paused") {
            continue;
        }
        let Some(revision) = wf.active_revision else {
            continue;
        };
        let row = match ledger.workflow_revision(&wf.name, revision) {
            Ok(row) => row,
            Err(error) => {
                eprintln!(
                    "workflow '{}' skipped: missing active revision ({error:#})",
                    wf.name
                );
                continue;
            }
        };
        let doc = match WorkflowDoc::from_canonical_json(&row.document)
            .and_then(|doc| doc.validate().map(|()| doc))
        {
            Ok(doc) => doc,
            Err(error) => {
                eprintln!(
                    "workflow '{}' skipped: invalid active revision ({error:#})",
                    wf.name
                );
                continue;
            }
        };
        for trigger in &doc.triggers {
            if trigger.kind != "webhook"
                || trigger
                    .route
                    .as_deref()
                    .map(crate::ingress::normalize_route)
                    .as_deref()
                    != Some(route.as_str())
            {
                continue;
            }
            let slot = if wf.state == "active" {
                &mut active
            } else {
                &mut paused
            };
            if let Some((owner, _)) = slot {
                bail!(
                    "webhook route '{route}' is claimed by active workflows '{}' and '{}'",
                    owner,
                    wf.name
                );
            }
            *slot = Some((wf.name.clone(), trigger.clone()));
        }
    }
    Ok(active.or(paused))
}

/// One accepted (or deduplicated) schedule fire, for logs and drills.
#[derive(Debug, Clone, Copy, PartialEq, Eq, Serialize)]
#[serde(rename_all = "snake_case")]
pub enum CronDisposition {
    Accepted,
    Duplicate,
    Suppressed,
    Denied,
}

#[derive(Debug, Serialize)]
pub struct CronAcceptance {
    pub workflow: String,
    pub scheduled: String,
    pub duplicate: bool,
    pub disposition: CronDisposition,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub detail: Option<String>,
}

/// Scan active workflows' cron triggers and accept fires due in
/// `(last, now]` through the normalized envelope, deduped on the scheduled
/// timestamp so restarts and overlapping windows never double-accept. At
/// most `max_fires` newest fires per workflow are accepted; skipped older
/// fires are recorded as a guard event, mirroring task cron collapse.
pub fn workflow_cron_tick(
    plane: &Plane,
    ledger: &Ledger,
    last_by_workflow: &mut HashMap<String, DateTime<Utc>>,
    default_last: DateTime<Utc>,
    now_utc: DateTime<Utc>,
    max_fires: u32,
) -> Result<Vec<CronAcceptance>> {
    let mut accepted = Vec::new();
    'workflow: for wf in ledger.list_workflows()? {
        if wf.state != "active" {
            continue;
        }
        let Some(revision) = wf.active_revision else {
            continue;
        };
        // Same skip-and-canary posture as webhook_workflow_target: a
        // poisoned sibling must not halt the whole cron tick every poll.
        let doc = match load_document(ledger, &wf.name, revision) {
            Ok(doc) => doc,
            Err(e) => {
                skip_unloadable_workflow(&wf.name, &e);
                continue;
            }
        };
        let last = *last_by_workflow
            .entry(wf.name.clone())
            .or_insert(default_last);
        let mut fires = Vec::new();
        let mut discarded = 0usize;
        let mut fired_total = 0usize;
        for trigger in &doc.triggers {
            if trigger.kind != "cron" {
                continue;
            }
            let schedule =
                match crate::ingress::parse_schedule(trigger.schedule.as_deref().unwrap_or("")) {
                    Ok(schedule) => schedule,
                    Err(error) => {
                        skip_unloadable_workflow(&wf.name, &error);
                        continue 'workflow;
                    }
                };
            let (bounded, skipped) = crate::ingress::due_fires_bounded_for_runtime(
                &schedule,
                last,
                now_utc,
                max_fires as usize,
            );
            fired_total = fired_total
                .saturating_add(bounded.len())
                .saturating_add(skipped);
            discarded = discarded.saturating_add(skipped);
            fires.extend(bounded);
        }
        fires.sort();
        let max = max_fires.max(1) as usize;
        if fires.len() > max {
            let skipped = fires.len() - max;
            discarded = discarded.saturating_add(skipped);
            fires = fires.split_off(skipped);
        }
        // Record every discarded fire from every trigger before advancing the
        // workflow cursor. Otherwise multi-trigger catch-up silently loses the
        // oldest per-trigger fires and a restart cannot explain the gap.
        if discarded > 0 {
            ledger.record_guard_event(
                "workflow_cron_collapse",
                Some(&wf.name),
                &format!("skipped={discarded} fired={fired_total}"),
                discarded as i64,
            )?;
        }
        for fire in fires {
            let scheduled = fire.to_rfc3339();
            let outcome = accept(
                plane,
                ledger,
                &TriggerEnvelope {
                    workflow: wf.name.clone(),
                    source: TriggerSource::Schedule,
                    payload: None,
                    dedupe_key: Some(format!("cron:{scheduled}")),
                },
            )?;
            let (disposition, detail, duplicate) = match outcome {
                AcceptOutcome::Accepted { .. } => (CronDisposition::Accepted, None, false),
                AcceptOutcome::Duplicate { .. } => (CronDisposition::Duplicate, None, true),
                AcceptOutcome::Suppressed { reason, .. } => {
                    (CronDisposition::Suppressed, Some(reason), false)
                }
                AcceptOutcome::Denied { kind, reason, .. } => (
                    CronDisposition::Denied,
                    Some(format!("{kind}: {reason}")),
                    false,
                ),
            };
            accepted.push(CronAcceptance {
                workflow: wf.name.clone(),
                scheduled,
                duplicate,
                disposition,
                detail,
            });
        }
        last_by_workflow.insert(wf.name.clone(), now_utc);
    }
    Ok(accepted)
}

/// Loud, non-fatal skip for enumeration paths (webhook target scan, cron
/// tick): the poisoned workflow is named on stderr and the canary; healthy
/// siblings keep working. Executing or rolling back the poisoned workflow
/// itself still refuses hard through [`load_document`].
fn skip_unloadable_workflow(name: &str, err: &anyhow::Error) {
    eprintln!("workflow '{name}' skipped (active revision unusable): {err:#}");
    crate::canary::report_error(
        "bb.workflow.doc",
        &format!("workflow '{name}' skipped: {err:#}"),
    );
}

/// The execution door: parse AND validate the pinned snapshot. A stored
/// revision that fails CURRENT validation — e.g. one written by an older
/// binary with weaker rules — must never execute; refusal carries the exact
/// reason. History stays readable through the raw-JSON view surfaces.
fn load_document(ledger: &Ledger, workflow: &str, revision: i64) -> Result<WorkflowDoc> {
    let row = ledger.workflow_revision(workflow, revision)?;
    let doc = WorkflowDoc::from_canonical_json(&row.document)?;
    doc.validate().with_context(|| {
        format!(
            "workflow '{workflow}' revision {revision} fails current validation and cannot execute"
        )
    })?;
    Ok(doc)
}

/// Reconstruct the executable workflow from verified activation rows while
/// retaining the canonical workflow-level grant from its pinned revision.
fn load_pinned_document(ledger: &Ledger, workflow: &str, revision: i64) -> Result<WorkflowDoc> {
    let workflow_row = ledger.workflow_by_name(workflow)?;
    let canonical_grant = load_document(ledger, workflow, revision)?.grant;
    let mut rows = ledger.require_verified_launch_snapshots(&workflow_row.id, revision)?;
    let mut snapshots = rows
        .drain(..)
        .map(|row| {
            let snapshot: LaunchSnapshot = serde_json::from_value(row.snapshot)
                .with_context(|| format!("launch snapshot for step '{}' is not valid JSON", row.step))?;
            if snapshot.workflow_id != workflow_row.id || snapshot.revision != revision || snapshot.step != row.step || snapshot.digest != row.digest {
                bail!("launch snapshot for step '{}' has mismatched workflow, revision, step, or digest", row.step);
            }
            snapshot.verify_digest()?;
            Ok(snapshot)
        })
        .collect::<Result<Vec<_>>>()?;
    snapshots.sort_by_key(|snapshot| snapshot.step_index);
    let first = snapshots
        .first()
        .context("verified launch snapshot set is empty")?;
    let steps = snapshots
        .iter()
        .map(|snapshot| {
            let composition = StepAgent {
                name: snapshot.name.clone(),
                version: snapshot.agent_revision,
                harness: snapshot.harness.clone(),
                model: snapshot.model.clone(),
                role: snapshot.role.clone(),
                bin: snapshot.bin.clone(),
                args: snapshot.args.clone(),
                provider: snapshot.provider.clone(),
                effort: snapshot.effort.clone(),
                skills: snapshot.skills.clone(),
                mcps: snapshot.mcps.clone(),
                tool_rules: snapshot.tool_rules.clone(),
                context_inputs: snapshot.context_inputs.clone(),
                fallbacks: snapshot.fallbacks.clone(),
                secrets: snapshot.secret_refs.clone(),
                bundle: snapshot.bundle.clone(),
            };
            let action = match snapshot.action.clone() {
                Some(WorkflowAction::Agent { results, .. }) => WorkflowAction::Agent {
                    goal: snapshot.step_goal.clone(),
                    composition,
                    results,
                },
                Some(action) => action,
                None => WorkflowAction::Agent {
                    goal: snapshot.step_goal.clone(),
                    composition,
                    results: Vec::new(),
                },
            };
            let grant = if snapshot.grant == Default::default() {
                crate::workflow::GrantSpec {
                    capabilities: snapshot.authority.iter().cloned().collect(),
                    ..Default::default()
                }
            } else {
                snapshot.grant.clone()
            };
            WorkflowStep {
                name: snapshot.step.clone(),
                grant,
                host: snapshot.host.clone(),
                repos: snapshot.repos.clone(),
                routes: snapshot.routes.clone(),
                authority_order: snapshot.authority.clone(),
                action,
            }
        })
        .collect();
    Ok(WorkflowDoc {
        name: workflow.to_string(),
        grant: canonical_grant,
        goal: first.workflow_goal.clone(),
        triggers: Vec::new(),
        steps,
        policies: WorkflowPolicies {
            timeout_minutes: first.timeout_minutes,
            max_runs_per_day: first.max_runs_per_day,
            max_cost_per_run_usd: first.max_cost_per_run_usd,
            max_cost_per_day_usd: first.max_cost_per_day_usd,
            estimated_cost_per_run_usd: first.estimated_cost_per_run_usd,
            side_effect_policy: first.side_effect_policy.clone(),
            concurrency: first.concurrency,
            substrate: first.substrate.clone(),
            max_rounds: first.max_rounds,
            max_elapsed_seconds: first.max_elapsed_seconds,
            seats: first.seats,
        },
    })
}

/// Load the activation-time launch contract. Runtime never resolves a mutable
/// Roster/catalog entry or reconstructs a composition from the desired TOML.
fn load_launch_snapshot(
    ledger: &Ledger,
    run: &crate::workflow::WorkflowRunRow,
    step: &str,
) -> Result<LaunchSnapshot> {
    let workflow = ledger.workflow_by_name(&run.workflow)?;
    let row = ledger
        .require_verified_launch_snapshots(&workflow.id, run.revision)?
        .into_iter()
        .find(|row| row.step == step)
        .with_context(|| format!("workflow '{}' revision {} has no launch snapshot for step '{}'; activate the revision before execution", run.workflow, run.revision, step))?;
    let snapshot: LaunchSnapshot = serde_json::from_value(row.snapshot)
        .with_context(|| format!("launch snapshot for step '{step}' is not valid JSON"))?;
    if snapshot.workflow_id != workflow.id
        || snapshot.revision != run.revision
        || snapshot.step != step
        || row.digest != snapshot.digest
    {
        bail!(
            "launch snapshot for step '{step}' has mismatched workflow, revision, step, or digest"
        );
    }
    snapshot.verify_digest()?;
    Ok(snapshot)
}
// --- run-group state --------------------------------------------------------

pub const RUN_STATES: &[&str] = &[
    "queued",
    "running",
    "succeeded",
    "failed",
    "incomplete",
    "stopped",
    "needs_attention",
];

fn workflow_transition_allowed(from: &str, to: &str) -> bool {
    from == to
        || matches!(
            (from, to),
            ("queued", "running")
                | (
                    "running",
                    "succeeded" | "failed" | "incomplete" | "stopped" | "needs_attention"
                )
                | ("needs_attention", "succeeded" | "failed" | "stopped")
        )
}

#[derive(Debug, Serialize)]
pub struct WorkflowRunStatusRow {
    pub run_id: String,
    pub state: String,
    pub detail: Option<String>,
    pub current_step: Option<String>,
    pub stop_requested: bool,
    pub stop_reason: Option<String>,
    /// Sum of OBSERVED step costs only; None means no step reported cost.
    /// Unknown is never laundered into zero.
    pub cost_usd: Option<f64>,
    pub started_at: Option<String>,
    pub updated_at: String,
}

#[derive(Clone, Debug, Serialize)]
pub struct StepRunRow {
    pub id: String,
    pub run_id: String,
    pub step: String,
    pub attempt: i64,
    pub agent: serde_json::Value,
    pub goal: String,
    pub state: String,
    pub outcome: Option<String>,
    pub summary: Option<String>,
    pub error: Option<String>,
    pub exit_code: Option<i64>,
    pub tokens_in: Option<i64>,
    pub tokens_out: Option<i64>,
    pub turns: Option<i64>,
    pub cost_usd: Option<f64>,
    pub artifact_dir: Option<String>,
    pub authority: Vec<String>,
    pub started_at: String,
    pub ended_at: Option<String>,
}

#[derive(Debug, Serialize)]
pub struct ChildAgentRow {
    pub id: i64,
    pub step_run_id: String,
    pub name: String,
    pub harness: Option<String>,
    pub model: Option<String>,
    pub goal: Option<String>,
    pub authority: Vec<String>,
    pub inherited: bool,
    pub cost_usd: Option<f64>,
    pub result: Option<String>,
    pub recorded_at: String,
}

fn row_to_status(r: &rusqlite::Row<'_>) -> rusqlite::Result<WorkflowRunStatusRow> {
    Ok(WorkflowRunStatusRow {
        run_id: r.get(0)?,
        state: r.get(1)?,
        detail: r.get(2)?,
        current_step: r.get(3)?,
        stop_requested: r.get::<_, i64>(4)? != 0,
        stop_reason: r.get(5)?,
        cost_usd: r.get(6)?,
        started_at: r.get(7)?,
        updated_at: r.get(8)?,
    })
}

const STATUS_SELECT: &str = "SELECT run_id, state, detail, current_step, stop_requested, \
     stop_reason, cost_usd, started_at, updated_at FROM workflow_run_status";

fn labels_json(labels: &[String]) -> String {
    serde_json::to_string(labels).expect("string vec serializes")
}

fn labels_from_json(text: &str) -> Vec<String> {
    serde_json::from_str(text).unwrap_or_default()
}

impl Ledger {
    pub fn workflow_run_status(&self, run_id: &str) -> Result<Option<WorkflowRunStatusRow>> {
        Ok(self
            .conn
            .query_row(
                &format!("{STATUS_SELECT} WHERE run_id = ?1"),
                params![run_id],
                row_to_status,
            )
            .optional()?)
    }

    /// Claim a queued run for execution. Ownership is durable and explicit:
    /// principal/claim identity is recorded before any worker effect runs.
    pub fn claim_workflow_run(&self, run_id: &str) -> Result<bool> {
        self.claim_workflow_run_for(run_id, "bb-controller", "controller", 3600)
    }

    pub fn claim_workflow_run_for(
        &self,
        run_id: &str,
        holder_principal: &str,
        claim_id: &str,
        lease_seconds: i64,
    ) -> Result<bool> {
        let ts = now();
        let expires = (time::OffsetDateTime::now_utc()
            + time::Duration::seconds(lease_seconds.max(1)))
        .format(&time::format_description::well_known::Rfc3339)
        .context("format workflow lease expiry")?;
        self.conn.execute(
            "INSERT OR IGNORE INTO workflow_run_status (run_id, state, updated_at)
             VALUES (?1, 'queued', ?2)",
            params![run_id, ts],
        )?;
        let updated = self.conn.execute(
            "UPDATE workflow_run_status
             SET state = 'running', started_at = ?2, updated_at = ?2,
                 holder_principal = ?3, claim_id = ?4, lease_expires_at = ?5
             WHERE run_id = ?1 AND state = 'queued'",
            params![run_id, ts, holder_principal, claim_id, expires],
        )?;
        Ok(updated == 1)
    }

    pub fn workflow_run_lease(&self, run_id: &str) -> Result<Option<LiveLease>> {
        self.conn
            .query_row(
                "SELECT run_id, holder_principal, claim_id, lease_expires_at
             FROM workflow_run_status
             WHERE run_id = ?1 AND state = 'running' AND holder_principal IS NOT NULL
               AND claim_id IS NOT NULL AND lease_expires_at IS NOT NULL",
                params![run_id],
                |r| {
                    Ok(LiveLease {
                        run_id: r.get(0)?,
                        holder_principal: r.get(1)?,
                        claim_id: r.get(2)?,
                        expires_at: r.get(3)?,
                    })
                },
            )
            .optional()
            .map_err(Into::into)
    }

    pub fn renew_workflow_run_lease(
        &self,
        run_id: &str,
        claim_id: &str,
        lease_seconds: i64,
    ) -> Result<bool> {
        let expires = (time::OffsetDateTime::now_utc()
            + time::Duration::seconds(lease_seconds.max(1)))
        .format(&time::format_description::well_known::Rfc3339)
        .context("format workflow lease expiry")?;
        Ok(self.conn.execute(
            "UPDATE workflow_run_status SET lease_expires_at = ?3, updated_at = ?4
             WHERE run_id = ?1 AND claim_id = ?2 AND state = 'running'",
            params![run_id, claim_id, expires, now()],
        )? == 1)
    }

    /// Release a just-claimed run back to `queued` after an admission
    /// recheck violation. The pressure is environmental (spend ceilings,
    /// sibling run-count consumption), not a defect of this run: deferring
    /// keeps the accepted work and its pinned reservation intact for a
    /// later runner tick instead of terminally destroying it. CAS on
    /// `running` so a concurrent terminal transition wins.
    pub fn defer_claimed_workflow_run(&self, run_id: &str, reason: &str) -> Result<bool> {
        let updated = self.conn.execute(
            "UPDATE workflow_run_status
             SET state = 'queued', detail = ?2, updated_at = ?3
             WHERE run_id = ?1 AND state = 'running'",
            params![run_id, reason, now()],
        )?;
        Ok(updated == 1)
    }

    pub fn set_workflow_run_state(
        &self,
        run_id: &str,
        state: &str,
        detail: Option<&str>,
    ) -> Result<()> {
        if !RUN_STATES.contains(&state) {
            bail!("unknown workflow run state '{state}'");
        }
        let current: String = self
            .conn
            .query_row(
                "SELECT state FROM workflow_run_status WHERE run_id = ?1",
                params![run_id],
                |r| r.get(0),
            )
            .optional()?
            .with_context(|| format!("workflow run status row missing for {run_id}"))?;
        if !workflow_transition_allowed(&current, state) {
            bail!("invalid workflow run transition {current} -> {state}");
        }
        self.conn.execute(
            "UPDATE workflow_run_status SET state = ?2, detail = ?3, updated_at = ?4
             WHERE run_id = ?1",
            params![run_id, state, detail, now()],
        )?;
        Ok(())
    }

    fn set_workflow_run_current_step(&self, run_id: &str, step: &str) -> Result<()> {
        self.conn.execute(
            "UPDATE workflow_run_status SET current_step = ?2, updated_at = ?3 WHERE run_id = ?1",
            params![run_id, step, now()],
        )?;
        Ok(())
    }

    fn add_workflow_run_cost(&self, run_id: &str, cost: f64) -> Result<()> {
        let cost = crate::ledger::validate_cost_value(cost, "workflow run observed cost")?;
        self.conn.execute(
            "UPDATE workflow_run_status
             SET cost_usd = COALESCE(cost_usd, 0) + ?2, updated_at = ?3
             WHERE run_id = ?1",
            params![run_id, cost, now()],
        )?;
        Ok(())
    }

    /// Record an external stop signal for a run group. Effective before the
    /// next step attempt; the in-flight attempt is never killed blindly
    /// because it may already have external side effects.
    pub fn request_workflow_run_stop(
        &self,
        run_id: &str,
        reason: &str,
    ) -> Result<WorkflowRunStatusRow> {
        // Touching the run proves it exists (workflow_run errors otherwise).
        self.workflow_run(run_id)?;
        let current = self
            .workflow_run_status(run_id)?
            .context("workflow run status row missing")?;
        if !matches!(current.state.as_str(), "queued" | "running") {
            bail!(
                "cannot request stop for workflow run {run_id} in state {}",
                current.state
            );
        }
        let ts = now();
        self.conn.execute(
            "UPDATE workflow_run_status
             SET stop_requested = 1, stop_reason = ?2, updated_at = ?3
             WHERE run_id = ?1",
            params![run_id, reason, ts],
        )?;
        self.workflow_run_status(run_id)?
            .context("workflow run status row missing after stop request")
    }

    fn workflow_run_stop_reason(&self, run_id: &str) -> Result<Option<String>> {
        let row: Option<(i64, Option<String>)> = self
            .conn
            .query_row(
                "SELECT stop_requested, stop_reason FROM workflow_run_status WHERE run_id = ?1",
                params![run_id],
                |r| Ok((r.get(0)?, r.get(1)?)),
            )
            .optional()?;
        Ok(match row {
            Some((1.., reason)) => Some(reason.unwrap_or_else(|| "stop requested".to_string())),
            _ => None,
        })
    }

    pub const WORKFLOW_QUEUE_BATCH: i64 = 64;

    pub fn queued_workflow_run_ids(&self) -> Result<Vec<String>> {
        let mut stmt = self.conn.prepare(
            "SELECT run_id FROM workflow_run_status
             WHERE state = 'queued' ORDER BY updated_at, run_id LIMIT ?1",
        )?;
        let ids = stmt
            .query_map(params![Self::WORKFLOW_QUEUE_BATCH], |r| r.get(0))?
            .collect::<rusqlite::Result<Vec<String>>>()?;
        Ok(ids)
    }

    pub fn workflow_freshness_summary(
        &self,
        stale_after_seconds: i64,
    ) -> Result<serde_json::Value> {
        let threshold =
            (Utc::now() - chrono::Duration::seconds(stale_after_seconds.max(0))).to_rfc3339();
        let queued: i64 = self.conn.query_row(
            "SELECT COUNT(*) FROM workflow_run_status WHERE state = 'queued'",
            [],
            |r| r.get(0),
        )?;
        let running: i64 = self.conn.query_row(
            "SELECT COUNT(*) FROM workflow_run_status WHERE state = 'running'",
            [],
            |r| r.get(0),
        )?;
        let needs_attention: i64 = self.conn.query_row(
            "SELECT COUNT(*) FROM workflow_run_status WHERE state = 'needs_attention'",
            [],
            |r| r.get(0),
        )?;
        let stale_running: i64 = self.conn.query_row(
            "SELECT COUNT(*) FROM workflow_run_status WHERE state = 'running' AND updated_at < ?1",
            params![threshold],
            |r| r.get(0),
        )?;
        let oldest_queued: Option<String> = self.conn.query_row(
            "SELECT MIN(updated_at) FROM workflow_run_status WHERE state = 'queued'",
            [],
            |r| r.get(0),
        )?;
        let oldest_running: Option<String> = self.conn.query_row(
            "SELECT MIN(updated_at) FROM workflow_run_status WHERE state = 'running'",
            [],
            |r| r.get(0),
        )?;
        Ok(serde_json::json!({
            "queued": queued,
            "running": running,
            "needs_attention": needs_attention,
            "stale_running": stale_running,
            "stale_after_seconds": stale_after_seconds.max(0),
            "oldest_queued_updated_at": oldest_queued,
            "oldest_running_updated_at": oldest_running,
        }))
    }

    fn running_workflow_run_ids(&self) -> Result<Vec<String>> {
        let mut stmt = self.conn.prepare(
            "SELECT DISTINCT s.run_id FROM workflow_run_status s
             WHERE s.state = 'running'
                OR (s.state = 'needs_attention' AND EXISTS (
                    SELECT 1 FROM workflow_step_runs wsr
                    WHERE wsr.run_id = s.run_id AND wsr.state = 'running'
                ))
             ORDER BY s.updated_at, s.run_id",
        )?;
        let ids = stmt
            .query_map([], |r| r.get(0))?
            .collect::<rusqlite::Result<Vec<String>>>()?;
        Ok(ids)
    }

    #[allow(clippy::too_many_arguments)]
    fn create_workflow_step_run(
        &self,
        run_id: &str,
        step: &str,
        attempt: i64,
        agent_json: &str,
        goal: &str,
        authority: &[String],
        artifact_dir: &str,
    ) -> Result<String> {
        let id = format!("wfs-{}", new_id());
        self.conn.execute(
            "INSERT INTO workflow_step_runs
               (id, run_id, step, attempt, agent_json, goal, state, artifact_dir,
                authority_json, started_at)
             VALUES (?1, ?2, ?3, ?4, ?5, ?6, 'running', ?7, ?8, ?9)",
            params![
                id,
                run_id,
                step,
                attempt,
                agent_json,
                goal,
                artifact_dir,
                labels_json(authority),
                now()
            ],
        )?;
        Ok(id)
    }

    #[allow(clippy::too_many_arguments)]
    fn finish_workflow_step_run(
        &self,
        id: &str,
        state: &str,
        outcome: Option<&str>,
        summary: Option<&str>,
        error: Option<&str>,
        exit_code: Option<i64>,
        stats: &AttemptStats,
    ) -> Result<()> {
        let cost = stats
            .cost_usd
            .map(|value| crate::ledger::validate_cost_value(value, "workflow step observed cost"))
            .transpose()?;
        self.conn.execute(
            "UPDATE workflow_step_runs
             SET state = ?2, outcome = ?3, summary = ?4, error = ?5, exit_code = ?6,
                 tokens_in = ?7, tokens_out = ?8, turns = ?9, cost_usd = ?10, ended_at = ?11
             WHERE id = ?1",
            params![
                id,
                state,
                outcome,
                summary,
                error,
                exit_code,
                stats.tokens_in,
                stats.tokens_out,
                stats.turns,
                cost,
                now()
            ],
        )?;
        Ok(())
    }

    /// Close any step rows an aborted executor left `running`: the executor
    /// error path uses this so evidence rows are never stranded in-flight.
    fn fail_running_step_runs(&self, run_id: &str, error: &str) -> Result<()> {
        self.conn.execute(
            "UPDATE workflow_step_runs SET state = 'failed', error = ?2, ended_at = ?3
             WHERE run_id = ?1 AND state = 'running'",
            params![run_id, error, now()],
        )?;
        Ok(())
    }

    /// Close a step row left running by a crash. Recovery intentionally writes
    /// a failure/uncertainty explanation instead of pretending the outcome is
    /// known; the run status carries the operator-facing disposition.
    pub fn close_workflow_step_for_recovery(&self, step_run_id: &str, error: &str) -> Result<()> {
        self.conn.execute(
            "UPDATE workflow_step_runs
             SET state = 'failed', error = ?2, ended_at = ?3
             WHERE id = ?1 AND state = 'running'",
            params![step_run_id, error, now()],
        )?;
        Ok(())
    }

    /// Record runtime evidence in the workflow audit trail without exposing the
    /// composition store's private transaction helper.
    pub fn record_workflow_runtime_event(
        &self,
        run_id: &str,
        kind: &str,
        data: Option<&str>,
    ) -> Result<()> {
        self.conn.execute(
            "INSERT INTO workflow_events (workflow_id, run_id, kind, data, at)
             SELECT workflow_id, id, ?2, ?3, ?4 FROM workflow_runs WHERE id = ?1",
            params![run_id, kind, data, now()],
        )?;
        Ok(())
    }

    /// Finished attempts of the named steps in this run group that reported
    /// no cost. The spend guard reads this — scoped to steps ON cycles — to
    /// distinguish "no spend observed yet" from "cycle spend happened but
    /// was never metered". Off-cycle steps are validation-admitted blind
    /// (they run a bounded number of times), so they never count here.
    fn unmetered_workflow_attempts(&self, run_id: &str, steps: &[&str]) -> Result<i64> {
        if steps.is_empty() {
            return Ok(0);
        }
        let placeholders = vec!["?"; steps.len()].join(", ");
        let sql = format!(
            "SELECT COUNT(*) FROM workflow_step_runs
             WHERE run_id = ? AND state != 'running' AND cost_usd IS NULL
               AND step IN ({placeholders})"
        );
        let mut stmt = self.conn.prepare(&sql)?;
        let params = rusqlite::params_from_iter(
            std::iter::once(run_id.to_string()).chain(steps.iter().map(|s| s.to_string())),
        );
        Ok(stmt.query_row(params, |r| r.get(0))?)
    }

    fn workflow_step_attempts(&self, run_id: &str, step: &str) -> Result<i64> {
        Ok(self.conn.query_row(
            "SELECT COUNT(*) FROM workflow_step_runs WHERE run_id = ?1 AND step = ?2",
            params![run_id, step],
            |r| r.get(0),
        )?)
    }

    fn next_workflow_attempt(&self, run_id: &str) -> Result<i64> {
        Ok(self.conn.query_row(
            "SELECT COALESCE(MAX(attempt), 0) + 1 FROM workflow_step_runs WHERE run_id = ?1",
            params![run_id],
            |r| r.get(0),
        )?)
    }

    pub fn workflow_step_runs(&self, run_id: &str) -> Result<Vec<StepRunRow>> {
        let mut stmt = self.conn.prepare(
            "SELECT id, run_id, step, attempt, agent_json, goal, state, outcome, summary,
                    error, exit_code, tokens_in, tokens_out, turns, cost_usd, artifact_dir,
                    authority_json, started_at, ended_at
             FROM workflow_step_runs WHERE run_id = ?1 ORDER BY attempt",
        )?;
        let rows = stmt
            .query_map(params![run_id], |r| {
                Ok(StepRunRow {
                    id: r.get(0)?,
                    run_id: r.get(1)?,
                    step: r.get(2)?,
                    attempt: r.get(3)?,
                    agent: serde_json::from_str(&r.get::<_, String>(4)?)
                        .unwrap_or(serde_json::Value::Null),
                    goal: r.get(5)?,
                    state: r.get(6)?,
                    outcome: r.get(7)?,
                    summary: r.get(8)?,
                    error: r.get(9)?,
                    exit_code: r.get(10)?,
                    tokens_in: r.get(11)?,
                    tokens_out: r.get(12)?,
                    turns: r.get(13)?,
                    cost_usd: r.get(14)?,
                    artifact_dir: r.get(15)?,
                    authority: labels_from_json(&r.get::<_, String>(16)?),
                    started_at: r.get(17)?,
                    ended_at: r.get(18)?,
                })
            })?
            .collect::<rusqlite::Result<Vec<_>>>()?;
        Ok(rows)
    }

    #[allow(clippy::too_many_arguments)]
    fn record_workflow_child_agent(
        &self,
        step_run_id: &str,
        name: &str,
        harness: Option<&str>,
        model: Option<&str>,
        goal: Option<&str>,
        authority: &[String],
        inherited: bool,
        cost_usd: Option<f64>,
        result: Option<&str>,
    ) -> Result<()> {
        self.conn.execute(
            "INSERT INTO workflow_child_agents
               (step_run_id, name, harness, model, goal, authority_json, inherited,
                cost_usd, result, recorded_at)
             VALUES (?1, ?2, ?3, ?4, ?5, ?6, ?7, ?8, ?9, ?10)",
            params![
                step_run_id,
                name,
                harness,
                model,
                goal,
                labels_json(authority),
                inherited as i64,
                cost_usd,
                result,
                now()
            ],
        )?;
        Ok(())
    }

    pub fn workflow_child_agents(&self, step_run_id: &str) -> Result<Vec<ChildAgentRow>> {
        let mut stmt = self.conn.prepare(
            "SELECT id, step_run_id, name, harness, model, goal, authority_json, inherited,
                    cost_usd, result, recorded_at
             FROM workflow_child_agents WHERE step_run_id = ?1 ORDER BY id",
        )?;
        let rows = stmt
            .query_map(params![step_run_id], |r| {
                Ok(ChildAgentRow {
                    id: r.get(0)?,
                    step_run_id: r.get(1)?,
                    name: r.get(2)?,
                    harness: r.get(3)?,
                    model: r.get(4)?,
                    goal: r.get(5)?,
                    authority: labels_from_json(&r.get::<_, String>(6)?),
                    inherited: r.get::<_, i64>(7)? != 0,
                    cost_usd: r.get(8)?,
                    result: r.get(9)?,
                    recorded_at: r.get(10)?,
                })
            })?
            .collect::<rusqlite::Result<Vec<_>>>()?;
        Ok(rows)
    }
}

// --- boot recovery -----------------------------------------------------------

#[derive(Debug, Serialize)]
pub struct WorkflowRecoveryReport {
    pub run_id: String,
    pub workflow: String,
    pub step: Option<String>,
    pub attempt_phase: Option<String>,
    pub probe: Option<String>,
    pub probe_state: Option<String>,
    pub probe_reason: Option<String>,
    pub lease_disposition: String,
    pub disposition: String,
}

fn workflow_recovery_report(
    run_id: String,
    workflow: String,
    step: Option<String>,
    disposition: &str,
    probe: Option<crate::substrate::ProbeResult>,
    lease_disposition: &str,
    attempt_phase: Option<String>,
) -> WorkflowRecoveryReport {
    let (probe_text, probe_state, probe_reason) = match probe {
        Some(probe) => (
            Some(probe.description()),
            Some(probe.state().to_string()),
            probe.reason().map(ToString::to_string),
        ),
        None => (None, None, None),
    };
    WorkflowRecoveryReport {
        run_id,
        workflow,
        step,
        attempt_phase,
        probe: probe_text,
        probe_state,
        probe_reason,
        lease_disposition: lease_disposition.to_string(),
        disposition: disposition.to_string(),
    }
}

/// Classify workflow runs inherited in running state at boot. Terminal step
/// evidence is reconciled first; an in-flight step is probed before recovery
/// closes its row. No path blindly re-executes work that may have side effects.
pub fn recover_inherited_workflow_runs(
    plane: &Plane,
    ledger: &Ledger,
) -> Result<Vec<WorkflowRecoveryReport>> {
    let mut reports = Vec::new();
    for run_id in ledger.running_workflow_run_ids()? {
        let run = ledger.workflow_run(&run_id)?;
        let status = ledger
            .workflow_run_status(&run_id)?
            .context("running workflow run has no status row")?;
        let steps = ledger.workflow_step_runs(&run_id)?;
        let mut running = steps.iter().filter(|step| step.state == "running");
        let current = running.next().cloned();
        let extra_running: Vec<_> = running.cloned().collect();
        for step in extra_running {
            ledger.close_workflow_step_for_recovery(
                &step.id,
                "recovery found more than one running step row; outcome is unknown",
            )?;
        }

        if let Some(step) = current {
            let probe = match load_document(ledger, &run.workflow, run.revision).and_then(|doc| {
                let spec = doc
                    .steps
                    .iter()
                    .find(|candidate| candidate.name == step.step)
                    .with_context(|| {
                        format!("current step '{}' missing from pinned document", step.step)
                    })?;
                let substrate_kind = doc.policies.substrate.as_deref().unwrap_or("local");
                let substrate = substrate::for_task(substrate_kind)?;
                let host = spec
                    .host
                    .clone()
                    .unwrap_or_else(|| format!("wf-{}", run.id));
                let attempt_dir = step
                    .artifact_dir
                    .as_deref()
                    .map(std::path::PathBuf::from)
                    .unwrap_or_else(|| {
                        plane
                            .root
                            .join(".bb/workflow-runs")
                            .join(&run.id)
                            .join(format!("attempt-{}-{}", step.attempt, step.step))
                    });
                Ok(substrate.probe(&host, &attempt_dir, &format!("bb-{}", step.id)))
            }) {
                Ok(probe) => probe,
                Err(error) => crate::substrate::ProbeResult::Unknown(error.to_string()),
            };
            let probe_desc = probe.description();
            ledger.record_workflow_runtime_event(&run.id, "boot_probe", Some(&probe_desc))?;
            match probe {
                crate::substrate::ProbeResult::Alive => {
                    let detail = format!(
                        "inherited workflow step '{}' at attempt {} is still live after restart;                          side effects unknown; inspect evidence before resolving",
                        step.step, step.attempt
                    );
                    ledger.set_workflow_run_state(&run.id, "needs_attention", Some(&detail))?;
                    reports.push(workflow_recovery_report(
                        run.id,
                        run.workflow,
                        Some(step.step),
                        "needs_attention",
                        Some(probe),
                        "retained",
                        Some("executing".to_string()),
                    ));
                }
                probe @ (crate::substrate::ProbeResult::Dead
                | crate::substrate::ProbeResult::Unknown(_)) => {
                    let detail = format!(
                        "inherited workflow step '{}' at attempt {} has {} — side effects unknown; \
                         inspect evidence before explicitly replaying",
                        step.step, step.attempt, probe_desc
                    );
                    ledger.close_workflow_step_for_recovery(&step.id, &detail)?;
                    ledger.set_workflow_run_state(&run.id, "needs_attention", Some(&detail))?;
                    let lease_disposition = if matches!(probe, crate::substrate::ProbeResult::Dead)
                    {
                        ledger.release_host_leases_for_run(&run.id)?;
                        "released"
                    } else {
                        "retained"
                    };
                    reports.push(workflow_recovery_report(
                        run.id,
                        run.workflow,
                        Some(step.step),
                        "needs_attention",
                        Some(probe),
                        lease_disposition,
                        Some("executing".to_string()),
                    ));
                }
            }
            continue;
        }

        let last = steps.last();
        let (disposition, detail) = match last {
            Some(step) if step.state == "succeeded" => {
                let terminal = load_document(ledger, &run.workflow, run.revision)
                    .ok()
                    .and_then(|doc| {
                        doc.steps
                            .into_iter()
                            .find(|candidate| candidate.name == step.step)
                    })
                    .map(|spec| {
                        let target = match step.outcome.as_deref() {
                            Some(outcome) => spec.routes.get(outcome).map(String::as_str),
                            None if spec.routes.len() <= 1 => {
                                spec.routes.values().next().map(String::as_str)
                            }
                            None => None,
                        };
                        spec.routes.is_empty() || target == Some(ROUTE_DONE)
                    })
                    .unwrap_or(false);
                if terminal {
                    ("succeeded", "recovered terminal workflow step evidence")
                } else {
                    (
                        "needs_attention",
                        "workflow step succeeded but route/finalization evidence is incomplete",
                    )
                }
            }
            Some(step) if step.state == "failed" => {
                ("failed", "recovered failed workflow step evidence")
            }
            Some(step) if step.state == "incomplete" => {
                ("incomplete", "recovered incomplete workflow step evidence")
            }
            Some(_) => (
                "needs_attention",
                "workflow has no running step but terminal evidence is ambiguous",
            ),
            None => (
                "needs_attention",
                "workflow run has no step evidence; outcome is unknown",
            ),
        };
        ledger.set_workflow_run_state(&run.id, disposition, Some(detail))?;
        ledger.release_host_leases_for_run(&run.id)?;
        reports.push(workflow_recovery_report(
            run.id,
            run.workflow,
            status.current_step,
            disposition,
            None,
            "released",
            None,
        ));
    }
    Ok(reports)
}

/// Resolve an inherited workflow run only after an operator has inspected
/// side effects. Lease release is keyed by run identity, so an unrelated run
/// cannot be stranded or released by this operation.
pub fn resolve_workflow_run(
    ledger: &Ledger,
    run_id: &str,
    state: &str,
    reason: &str,
) -> Result<WorkflowRunStatusRow> {
    if !matches!(state, "succeeded" | "failed" | "stopped") {
        bail!("workflow recovery resolution state must be succeeded, failed, or stopped");
    }
    let current = ledger
        .workflow_run_status(run_id)?
        .context("workflow run status row missing")?;
    if current.state != "needs_attention" {
        bail!(
            "workflow run {run_id} is {}, not needs_attention; resolve only recovered uncertainty",
            current.state
        );
    }
    // A live probe is still an uncertain execution outcome. Resolve the
    // operator-visible step row before the terminal run transition so no
    // recovered attempt remains falsely in flight after resolution.
    let step_detail = format!("operator resolved recovered step: {reason}");
    for step in ledger.workflow_step_runs(run_id)? {
        if step.state == "running" {
            ledger.close_workflow_step_for_recovery(&step.id, &step_detail)?;
        }
    }
    ledger.release_host_leases_for_run(run_id)?;
    ledger.set_workflow_run_state(run_id, state, Some(reason))?;
    ledger
        .workflow_run_status(run_id)?
        .context("workflow run status row missing after resolution")
}

// --- execution ---------------------------------------------------------------

/// The completion tool's file contract. `outcome` must be one of the step's
/// declared route outcomes when the step is branching.
#[derive(Debug, Deserialize)]
#[serde(deny_unknown_fields)]
struct CompletionDoc {
    #[serde(default)]
    outcome: Option<String>,
    #[serde(default)]
    summary: Option<String>,
    /// Optional artifact/receipt references. Accepted (deny_unknown_fields
    /// would otherwise reject the documented contract) and retained in the
    /// collected OUTCOME.json evidence rather than a separate column.
    #[serde(default)]
    #[allow(dead_code)]
    artifacts: Vec<String>,
}

/// One declared dynamic child agent (evidence, not a catalog entry).
#[derive(Debug, Deserialize)]
#[serde(deny_unknown_fields)]
struct ChildDecl {
    name: String,
    #[serde(default)]
    harness: Option<String>,
    #[serde(default)]
    model: Option<String>,
    #[serde(default)]
    goal: Option<String>,
    /// Absent = inherit the parent step grant verbatim. Present = must be a
    /// subset of the parent grant (authority narrows monotonically).
    #[serde(default)]
    authority: Option<Vec<String>>,
    #[serde(default)]
    cost_usd: Option<f64>,
    #[serde(default)]
    result: Option<String>,
}

enum StepDisposition {
    Succeeded {
        outcome: Option<String>,
        summary: Option<String>,
    },
    /// The launch never reached adapter execution; an ordered fallback may
    /// consume this bounded attempt without replaying post-start work.
    PreExecFailed(String),
    Failed(String),
    Incomplete(String),
    Stopped(String),
    NeedsAttention(String),
}

struct WorkflowInFlightMonitor<'a> {
    ledger: &'a Ledger,
    run_id: &'a str,
    workflow: &'a str,
    harness: &'a str,
    max_cost_usd: f64,
    prior_cost_usd: f64,
    side_effect_policy: &'a str,
    last_recorded_cost: Option<f64>,
    breached: bool,
}

impl<'a> WorkflowInFlightMonitor<'a> {
    fn observe(&mut self, snapshot: &ExecSnapshot<'_>) -> Option<String> {
        let progress = harness::parse_partial_progress(self.harness, snapshot.stdout);
        let cost = progress.stats.cost_usd?;
        let total = self.prior_cost_usd + cost;
        let changed = self
            .last_recorded_cost
            .is_none_or(|prior| (prior - cost).abs() > f64::EPSILON);
        if changed {
            let _ = self.ledger.record_progress(
                self.run_id,
                &format!(
                    "workflow cost observed ${cost:.4} (run total ${total:.4}) elapsed={}s",
                    snapshot.elapsed.as_secs()
                ),
            );
            self.last_recorded_cost = Some(cost);
        }
        if total < self.max_cost_usd || self.breached {
            return None;
        }
        let reason = format!(
            "workflow in-flight cost cap {}: observed run total ${total:.4} (current step ${cost:.4} + prior ${prior:.4}) >= max_cost_per_run_usd ${max:.2}",
            self.side_effect_policy, prior = self.prior_cost_usd, max = self.max_cost_usd
        );
        let _ = self.ledger.record_guard_event(
            "workflow_guard_spend_in_flight",
            Some(self.workflow),
            &reason,
            1,
        );
        self.breached = true;
        match self.side_effect_policy {
            "log" => None,
            _ => Some(reason),
        }
    }
}

/// Outcome of one execution attempt on a queued workflow run.
pub enum ExecutionDisposition {
    /// The run was claimed and driven to a terminal state.
    Executed(serde_json::Value),
    /// The queued->running claim was lost: someone else is executing.
    ClaimLost,
    /// The admission recheck refused capacity; the run was released back
    /// to `queued` (reservation intact) for a later runner tick.
    Deferred { reason: String },
}

/// Execute one accepted workflow run to a terminal state. Claims the run
/// (queued -> running CAS); refuses anything already claimed or terminal.
pub fn execute_run(plane: &Plane, ledger: &Ledger, run_id: &str) -> Result<serde_json::Value> {
    match execute_if_queued(plane, ledger, run_id)? {
        ExecutionDisposition::Executed(view) => Ok(view),
        ExecutionDisposition::Deferred { reason } => {
            bail!(
                "workflow run {run_id} deferred by admission recheck: {reason}; \
                 run remains queued"
            );
        }
        ExecutionDisposition::ClaimLost => {
            let state = ledger
                .workflow_run_status(run_id)?
                .map(|s| s.state)
                .unwrap_or_else(|| "unknown".to_string());
            bail!("workflow run {run_id} is {state}; only queued runs execute");
        }
    }
}

/// Runner-loop variant of [`execute_run`]: reports a lost claim or an
/// admission deferral as data, which the serve loop treats as non-events
/// instead of errors.
pub fn execute_if_queued(
    plane: &Plane,
    ledger: &Ledger,
    run_id: &str,
) -> Result<ExecutionDisposition> {
    let run = ledger.workflow_run(run_id)?;
    if !ledger.claim_workflow_run(run_id)? {
        return Ok(ExecutionDisposition::ClaimLost);
    }
    if let Some((kind, reason)) = ledger.recheck_workflow_run_admission(plane, run_id)? {
        // A pending operator stop outranks deferral: honor it terminally
        // (releasing the pinned reservation) instead of parking the run
        // behind capacity pressure forever.
        if let Some(stop) = ledger.workflow_run_stop_reason(run_id)? {
            let detail = format!("stop signal: {stop}");
            ledger.record_guard_event(
                "workflow_guard_stop_signal",
                Some(&run.workflow),
                &detail,
                1,
            )?;
            ledger.set_workflow_run_state(run_id, "stopped", Some(&detail))?;
            return Ok(ExecutionDisposition::Executed(run_detail_view(
                ledger, run_id,
            )?));
        }
        // Guard-event once per distinct reason, not once per runner tick:
        // the previous deferral left the same reason in the status detail.
        let already_deferred = ledger
            .workflow_run_status(run_id)?
            .and_then(|s| s.detail)
            .is_some_and(|detail| detail == reason);
        ledger.defer_claimed_workflow_run(run_id, &reason)?;
        if !already_deferred {
            ledger.record_guard_event(
                &format!("workflow_admission_recheck_{kind}"),
                Some(&run.workflow),
                &reason,
                1,
            )?;
        }
        return Ok(ExecutionDisposition::Deferred { reason });
    }
    // A claimed run must reach a terminal state even when the executor
    // itself errors (unreadable pinned doc, fs failure) OR panics: both
    // become `failed` with the reason recorded, never a phantom `running`,
    // and a panic never unwinds into the caller's thread.
    let result = std::panic::catch_unwind(std::panic::AssertUnwindSafe(|| {
        execute_claimed(plane, ledger, &run)
    }))
    .unwrap_or_else(|panic| {
        let msg = panic
            .downcast_ref::<&str>()
            .map(|s| s.to_string())
            .or_else(|| panic.downcast_ref::<String>().cloned())
            .unwrap_or_else(|| "non-string panic payload".to_string());
        Err(anyhow::anyhow!("executor panic: {msg}"))
    });
    match result {
        Ok(view) => Ok(ExecutionDisposition::Executed(view)),
        Err(err) => {
            let detail = format!("executor error: {err:#}");
            let _ = ledger.fail_running_step_runs(run_id, &detail);
            let _ = ledger.set_workflow_run_state(run_id, "failed", Some(&detail));
            Err(err)
        }
    }
}

fn execute_claimed(
    plane: &Plane,
    ledger: &Ledger,
    run: &crate::workflow::WorkflowRunRow,
) -> Result<serde_json::Value> {
    let run_id = run.id.as_str();
    crate::workflow::verified_activation_for_run(ledger, run)?;
    let doc = load_pinned_document(ledger, &run.workflow, run.revision)?;
    let started = Instant::now();

    let mut current = doc.steps[0].name.clone();
    let terminal: (&str, Option<String>, &str) = loop {
        let step = doc
            .steps
            .iter()
            .find(|s| s.name == current)
            .with_context(|| format!("route target '{current}' missing from pinned document"))?;

        // Absolute attempt ceiling: pure defense-in-depth BEHIND the declared
        // guards (validation already requires an enforceable one for cycles).
        // Not config — a run group needing this many step attempts is
        // pathological regardless of what its guards claim.
        let next_attempt = ledger.next_workflow_attempt(run_id)?;
        if next_attempt > MAX_RUN_GROUP_ATTEMPTS {
            let detail = format!(
                "absolute attempt ceiling: run group reached {MAX_RUN_GROUP_ATTEMPTS} step \
                 attempts; stopped as defense-in-depth"
            );
            ledger.record_guard_event(
                "workflow_guard_attempt_ceiling",
                Some(&run.workflow),
                &detail,
                1,
            )?;
            break ("stopped", Some(detail), "workflow_run_stopped");
        }

        let snapshot = load_launch_snapshot(ledger, run, &step.name)?;
        // Guards, checked before every attempt; first fired wins. Identity and
        // cost capability come from the immutable selected snapshot, never the
        // mutable desired declaration.
        if let Some(guard) = fired_guard(ledger, run, &doc, step, &snapshot, started)? {
            break ("stopped", Some(guard.clone()), "workflow_run_stopped");
        }

        let disposition = run_step(plane, ledger, run, &doc, step, &snapshot, started)?;
        // run_step_once performs the selected snapshot's terminal spend check.
        // The post-attempt guard is retained for a successful adapter result
        // that did not pass through that terminal branch, without replacing a
        // more precise selected-attempt stop reason.
        if matches!(&disposition, StepDisposition::Succeeded { .. }) {
            if let Some(reason) = post_attempt_spend_guard(ledger, run, &snapshot)? {
                break ("stopped", Some(reason), "workflow_run_stopped");
            }
        }
        match disposition {
            StepDisposition::PreExecFailed(error) => {
                break ("failed", Some(error), "workflow_run_failed");
            }
            StepDisposition::Failed(error) => {
                break ("failed", Some(error), "workflow_run_failed");
            }
            StepDisposition::Incomplete(reason) => {
                break ("incomplete", Some(reason), "workflow_run_incomplete");
            }
            StepDisposition::Stopped(reason) => {
                break ("stopped", Some(reason), "workflow_run_stopped");
            }
            StepDisposition::NeedsAttention(reason) => {
                break ("needs_attention", Some(reason), "workflow_needs_attention");
            }
            StepDisposition::Succeeded { outcome, summary } => {
                let target = match step.routes.len() {
                    0 => ROUTE_DONE.to_string(),
                    1 => step.routes.values().next().expect("len checked").clone(),
                    _ => {
                        // run_step guarantees both; Err (not panic) keeps the
                        // "never strands in running" invariant if it ever lies.
                        let outcome = outcome.as_deref().context(
                            "branching success without a declared outcome (executor invariant)",
                        )?;
                        step.routes
                            .get(outcome)
                            .with_context(|| {
                                format!(
                                    "outcome '{outcome}' not among declared routes \
                                     (executor invariant)"
                                )
                            })?
                            .clone()
                    }
                };
                if target == ROUTE_DONE {
                    break ("succeeded", summary, "");
                }
                current = target;
            }
        }
    };

    let (state, detail, notify_event) = terminal;
    ledger.set_workflow_run_state(run_id, state, detail.as_deref())?;
    if !notify_event.is_empty() {
        crate::notify::notify(
            plane,
            ledger,
            notify_event,
            &serde_json::json!({
                "workflow_run_id": run_id,
                "workflow": run.workflow,
                "state": state,
                "detail": detail,
            }),
        );
    }
    run_detail_view(ledger, run_id)
}

/// Evaluate the run group's guards for the next attempt of `step`. Returns
/// the fired guard description (also recorded as a guard event) or None.
fn post_attempt_spend_guard(
    ledger: &Ledger,
    run: &crate::workflow::WorkflowRunRow,
    snapshot: &LaunchSnapshot,
) -> Result<Option<String>> {
    let Some(cap) = snapshot.max_cost_per_run_usd else {
        return Ok(None);
    };
    let observed = ledger
        .workflow_run_status(&run.id)?
        .and_then(|status| status.cost_usd);
    let Some(observed) = observed else {
        return Ok(None);
    };
    if !observed.is_finite() || observed < 0.0 {
        let detail = format!(
            "spend guard invalid: observed cost {observed:?} is not finite and non-negative"
        );
        ledger.record_guard_event(
            "workflow_guard_spend_invalid",
            Some(&run.workflow),
            &detail,
            1,
        )?;
        return Ok(Some(detail));
    }
    if observed > cap {
        let detail =
            format!("spend guard: observed ${observed:.4} exceeds run-group cap ${cap:.2}");
        ledger.record_guard_event("workflow_guard_spend", Some(&run.workflow), &detail, 1)?;
        return Ok(Some(detail));
    }
    Ok(None)
}

fn fired_guard(
    ledger: &Ledger,
    run: &crate::workflow::WorkflowRunRow,
    doc: &WorkflowDoc,
    step: &WorkflowStep,
    snapshot: &LaunchSnapshot,
    started: Instant,
) -> Result<Option<String>> {
    let fired = if let Some(reason) = ledger.workflow_run_stop_reason(&run.id)? {
        Some((
            "workflow_guard_stop_signal",
            format!("stop signal: {reason}"),
        ))
    } else if let Some(max) = snapshot.max_rounds {
        let attempts = ledger.workflow_step_attempts(&run.id, &step.name)?;
        (attempts >= max as i64).then(|| {
            (
                "workflow_guard_rounds",
                format!(
                    "rounds guard: step '{}' already attempted {attempts} of max {max}",
                    step.name
                ),
            )
        })
    } else {
        None
    };
    // Rounds and stop are per-branch above; elapsed and spend apply always.
    let fired = match fired {
        Some(f) => Some(f),
        None => {
            if let Some(max) = snapshot.max_elapsed_seconds {
                if started.elapsed() >= Duration::from_secs(max) {
                    Some((
                        "workflow_guard_elapsed",
                        format!(
                            "elapsed guard: run group at {}s of max {max}s",
                            started.elapsed().as_secs()
                        ),
                    ))
                } else {
                    None
                }
            } else {
                None
            }
        }
    };
    let fired = match fired {
        Some(f) => Some(f),
        None => {
            if let Some(cap) = snapshot.max_cost_per_run_usd {
                let estimate_violation = if !harness::reports_cost(&snapshot.harness) {
                    let estimate = snapshot
                        .estimated_cost_per_run_usd
                        .or_else(|| snapshot.max_cost_per_run_usd.filter(|value| *value != 0.0))
                        .unwrap_or(1.0);
                    if !estimate.is_finite() || estimate <= 0.0 {
                        bail!(
                            "workflow '{}' has invalid conservative estimate {}",
                            run.workflow,
                            estimate
                        );
                    }
                    (estimate > cap).then(|| (
                        "workflow_guard_spend_estimate",
                        format!("spend estimate: cost-blind harness '{}' reserves ${:.4} > max_cost_per_run_usd ${:.2}; unknown spend is never treated as zero", snapshot.harness, estimate, cap),
                    ))
                } else {
                    None
                };
                let spend_is_only_cycle_guard =
                    snapshot.max_rounds.is_none() && snapshot.max_elapsed_seconds.is_none();
                let unmetered = if spend_is_only_cycle_guard {
                    let cycle_steps: Vec<&str> = doc
                        .steps_on_cycles()
                        .into_iter()
                        .map(|s| s.name.as_str())
                        .collect();
                    ledger.unmetered_workflow_attempts(&run.id, &cycle_steps)?
                } else {
                    0
                };
                let unknown_violation = (unmetered > 0).then(|| (
                    "workflow_guard_spend_indeterminate",
                    format!("spend guard indeterminate: {} cycle-step attempt(s) reported no cost and max_cost_per_run_usd is the only cycle guard — unknown spend is never treated as zero", unmetered),
                ));
                if estimate_violation.is_some() {
                    estimate_violation
                } else if unknown_violation.is_some() {
                    unknown_violation
                } else {
                    let observed = ledger
                        .workflow_run_status(&run.id)?
                        .and_then(|s| s.cost_usd)
                        .unwrap_or(0.0);
                    (observed >= cap).then(|| {
                        (
                            "workflow_guard_spend",
                            format!(
                                "spend guard: observed ${:.4} of run-group cap ${:.2}",
                                observed, cap
                            ),
                        )
                    })
                }
            } else {
                None
            }
        }
    };
    let Some((kind, detail)) = fired else {
        return Ok(None);
    };
    ledger.record_guard_event(kind, Some(&run.workflow), &detail, 1)?;
    Ok(Some(detail))
}

fn agent_spec_from(snapshot: &LaunchSnapshot) -> AgentSpec {
    AgentSpec {
        version: snapshot.agent_revision,
        harness: snapshot.harness.clone(),
        model: snapshot.model.clone(),
        role: snapshot.role.clone(),
        skills: snapshot.skills.clone(),
        provider: snapshot.provider.clone(),
        auth: None,
        bin: snapshot.bin.clone(),
        args: snapshot.args.clone(),
        secrets: snapshot.secret_refs.clone(),
        checkout_secrets: Vec::new(),
        optional_secrets: Vec::new(),
        policy: Default::default(),
        roster: None,
    }
}
/// Roster v0.2 resolved bundle consumption: the bundle's AGENTS.md joins the
/// commission and its digest is recorded as provenance. A declared-but-
/// missing bundle fails honestly instead of launching a different agent.
fn load_bundle(snapshot: &LaunchSnapshot) -> Result<Option<(String, String)>> {
    let Some(bundle) = &snapshot.bundle else {
        return Ok(None);
    };
    let path = PathBuf::from(bundle).join("AGENTS.md");
    let text = std::fs::read_to_string(&path)
        .with_context(|| format!("roster bundle AGENTS.md at {}", path.display()))?;
    use sha2::{Digest, Sha256};
    let digest = format!("{:x}", Sha256::digest(text.as_bytes()));
    if snapshot.bundle_digest.as_deref() != Some(digest.as_str()) {
        bail!(
            "roster bundle digest changed for agent {}: expected {}, found {}",
            snapshot.name,
            snapshot.bundle_digest.as_deref().unwrap_or("-"),
            digest,
        );
    }
    Ok(Some((text, digest)))
}
/// The step commission: workflow + step goals plus the mechanical completion
/// contract projected from config (declared outcomes, authority labels).
/// Projection, not judgment — every semantic token here comes from the
/// pinned document.
fn commission(
    doc: &WorkflowDoc,
    step: &WorkflowStep,
    snapshot: &LaunchSnapshot,
    bundle_agents_md: Option<&str>,
) -> String {
    let mut card = String::new();
    if let Some(agents_md) = bundle_agents_md {
        card.push_str(agents_md.trim_end());
        card.push_str("\n\n---\n\n");
    }
    card.push_str(&format!(
        "# Workflow step commission\n\nWorkflow: {} — {}\nStep: {}\n\n{}\n",
        doc.name,
        doc.goal,
        step.name,
        step.goal()
    ));
    card.push_str("\n## Resolved composition (activation snapshot)\n```json\n");
    card.push_str(&serde_json::to_string_pretty(snapshot).expect("snapshot serializes"));
    card.push_str("\n```\n");
    card.push_str("\n## Completion contract\n");
    if step.routes.len() >= 2 {
        let outcomes: Vec<&str> = step.routes.keys().map(String::as_str).collect();
        card.push_str(&format!(
            "This step routes on a declared outcome. Before finishing, write `{OUTCOME_FILENAME}` \
             in this directory as JSON: {{\"outcome\": <one of: {}>, \"summary\": \"<what happened \
             and why>\", \"artifacts\": []}}. The outcome MUST be exactly one of the declared \
             values; a missing or undeclared outcome leaves this step incomplete.\n",
            outcomes.join(", ")
        ));
    } else {
        card.push_str(
            "Successful completion of this step is sufficient; no outcome file is required. \
             You MAY write `OUTCOME.json` with a {\"summary\": \"...\"} for the record.\n",
        );
    }
    card.push_str(&format!(
        "\n## Child agents\nIf you commission child agents, record each in \
         `{CHILD_AGENTS_FILENAME}` as a JSON array: [{{\"name\", \"harness\", \"model\", \
         \"goal\", \"authority\": [...], \"cost_usd\", \"result\"}}]. Your authority grant is \
         {:?}; a child inherits it unless it declares a narrower subset. A child may never \
         broaden it.\n",
        step.grant.capabilities
    ));
    card
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

fn finish_attempt(
    ledger: &Ledger,
    run_id: &str,
    step_run_id: &str,
    state: &str,
    error: Option<&str>,
    exit_code: Option<i64>,
    stats: &AttemptStats,
) -> Result<()> {
    if let Some(cost) = stats.cost_usd {
        ledger.add_workflow_run_cost(run_id, cost)?;
    }
    ledger.finish_workflow_step_run(step_run_id, state, None, None, error, exit_code, stats)
}

fn final_spend_breach(
    ledger: &Ledger,
    run: &crate::workflow::WorkflowRunRow,
    max_cost: f64,
    side_effect_policy: &str,
    current_cost: f64,
) -> Result<Option<String>> {
    let total = ledger
        .workflow_run_status(&run.id)?
        .and_then(|status| status.cost_usd)
        .unwrap_or(current_cost);
    if total < max_cost {
        return Ok(None);
    }
    Ok(Some(format!(
        "spend guard: observed run total ${:.4} (workflow final cost cap {}: current step ${:.4}) >= max_cost_per_run_usd ${:.2}", total, side_effect_policy, current_cost, max_cost
    )))
}

struct WorkflowLease<'a> {
    ledger: &'a Ledger,
    host: &'a str,
    run_id: &'a str,
}

impl Drop for WorkflowLease<'_> {
    fn drop(&mut self) {
        let _ = self.ledger.release_host_lease(self.host, self.run_id);
    }
}

fn run_step(
    plane: &Plane,
    ledger: &Ledger,
    run: &crate::workflow::WorkflowRunRow,
    doc: &WorkflowDoc,
    step: &WorkflowStep,
    snapshot: &LaunchSnapshot,
    started: Instant,
) -> Result<StepDisposition> {
    for index in 0..=snapshot.fallbacks.len() {
        let resolved = snapshot.resolve_fallback(index)?;
        match run_step_once(plane, ledger, run, doc, step, &resolved)? {
            StepDisposition::PreExecFailed(error) if index < snapshot.fallbacks.len() => {
                let next = index + 1;
                let resolved = snapshot.resolve_fallback(next)?;
                if let Some(reason) = fired_guard(ledger, run, doc, step, &resolved, started)? {
                    return Ok(StepDisposition::Stopped(reason));
                }
                ledger.record_guard_event(
                    "workflow_fallback_selected",
                    Some(&run.workflow),
                    &format!(
                        "step '{}' launch failed at composition index {index}; selected fallback index {next} digest {}: {error}",
                        step.name, resolved.digest
                    ),
                    1,
                )?;
            }
            StepDisposition::PreExecFailed(error) => return Ok(StepDisposition::Failed(error)),
            disposition => return Ok(disposition),
        }
    }
    unreachable!("fallback loop always returns a disposition")
}

fn run_step_once(
    plane: &Plane,
    ledger: &Ledger,
    run: &crate::workflow::WorkflowRunRow,
    doc: &WorkflowDoc,
    step: &WorkflowStep,
    snapshot: &LaunchSnapshot,
) -> Result<StepDisposition> {
    let attempt = ledger.next_workflow_attempt(&run.id)?;
    let attempt_dir = plane
        .root
        .join(".bb/workflow-runs")
        .join(&run.id)
        .join(format!("attempt-{attempt}-{}", step.name));
    std::fs::create_dir_all(&attempt_dir)?;
    let agent_json = serde_json::to_string(snapshot)?;
    let step_run_id = ledger.create_workflow_step_run(
        &run.id,
        &step.name,
        attempt,
        &agent_json,
        step.goal(),
        &snapshot.authority,
        &attempt_dir.to_string_lossy(),
    )?;
    ledger.set_workflow_run_current_step(&run.id, &step.name)?;

    let fail_terminal = |error: String, stats: &AttemptStats| -> Result<StepDisposition> {
        ledger.finish_workflow_step_run(
            &step_run_id,
            "failed",
            None,
            None,
            Some(&error),
            None,
            stats,
        )?;
        Ok(StepDisposition::Failed(format!(
            "step '{}' attempt {attempt}: {error}",
            step.name
        )))
    };
    let fail_pre_exec = |error: String| -> Result<StepDisposition> {
        ledger.finish_workflow_step_run(
            &step_run_id,
            "failed",
            None,
            None,
            Some(&error),
            None,
            &AttemptStats::default(),
        )?;
        Ok(StepDisposition::PreExecFailed(format!(
            "step '{}' attempt {attempt}: {error}",
            step.name
        )))
    };
    let fail_stopped = |error: String, stats: &AttemptStats| -> Result<StepDisposition> {
        ledger.finish_workflow_step_run(
            &step_run_id,
            "failed",
            None,
            None,
            Some(&error),
            None,
            stats,
        )?;
        ledger.record_guard_event(
            "workflow_guard_spend_invalid",
            Some(&run.workflow),
            &error,
            1,
        )?;
        Ok(StepDisposition::Stopped(format!(
            "step '{}' attempt {attempt}: {error}",
            step.name
        )))
    };
    let none = AttemptStats::default();
    match &step.action {
        WorkflowAction::Agent { .. } => {}
        WorkflowAction::Effect { .. } => {
            return fail_pre_exec(
                "action 'effect' denied before execution: no controller/effect executor exists yet"
                    .to_string(),
            )
        }
        WorkflowAction::Approval { .. } => {
            return fail_pre_exec(
                "action 'approval' denied before execution: no approval executor exists yet"
                    .to_string(),
            )
        }
    }
    if let Some(seats) = snapshot.seats {
        return fail_terminal(
            format!("policies.seats={seats} is unsupported; seat admission is not enforceable"),
            &none,
        );
    }
    if let Some(v) = budget::metered_parent_key_violation(
        &snapshot.name,
        &snapshot.harness,
        snapshot.provider.as_deref().unwrap_or("openrouter"),
        &snapshot.secret_refs,
        &[],
        None,
        None,
    ) {
        return fail_pre_exec(v.detail);
    }
    let stop = |reason: String| -> Result<StepDisposition> {
        ledger.finish_workflow_step_run(
            &step_run_id,
            "incomplete",
            None,
            None,
            Some(&reason),
            None,
            &none,
        )?;
        Ok(StepDisposition::Stopped(format!(
            "step '{}' attempt {attempt}: {reason}",
            step.name
        )))
    };

    let bundle = match load_bundle(snapshot) {
        Ok(b) => b,
        Err(e) => return fail_pre_exec(format!("{e:#}")),
    };
    let agent = agent_spec_from(snapshot);
    let hermetic = match agent.auth_class() {
        Ok(AuthClass::Api) => true,
        Ok(AuthClass::Subscription) => false,
        Err(e) => return fail_pre_exec(format!("{e:#}")),
    };
    let budget = TaskBudget {
        timeout_minutes: Some(
            snapshot
                .timeout_minutes
                .unwrap_or(DEFAULT_STEP_TIMEOUT_MINUTES),
        ),
        ..Default::default()
    };
    let cmd = match harness::build_command(&agent, &budget) {
        Ok(c) => c,
        Err(e) => return fail_pre_exec(format!("build_command: {e:#}")),
    };
    let mut secrets = Vec::new();
    for name in &snapshot.secret_refs {
        match std::env::var(name) {
            Ok(value) => secrets.push((name.clone(), value)),
            Err(_) => return fail_pre_exec(format!("secret env var '{name}' not set")),
        }
    }

    let substrate_kind = doc.policies.substrate.as_deref().unwrap_or("local");
    if substrate_kind == "local" && !plane.spec.dev && !plane.spec.allow_local_substrate {
        return fail_pre_exec(
            "substrate 'local' requires `allow_local_substrate = true` in production \
             or `dev = true` for development/test"
                .to_string(),
        );
    }
    let substrate = match substrate::for_task(substrate_kind) {
        Ok(s) => s,
        Err(e) => return fail_pre_exec(format!("{e:#}")),
    };
    let host = step
        .host
        .clone()
        .unwrap_or_else(|| format!("wf-{}", run.id));
    // Workflow runs sharing a declared host serialize, but admission must wait
    // for the bounded lease window rather than failing a valid concurrent run
    // before its isolated workspace is created.
    let lease_wait = WORKFLOW_LEASE_WAIT;
    let lease_started = Instant::now();
    loop {
        if let Some(reason) = ledger.workflow_run_stop_reason(&run.id)? {
            return stop(format!(
                "stop signal while waiting for host lease '{host}': {reason}"
            ));
        }
        if ledger.try_acquire_host_lease(&host, &run.id)? {
            if let Some(reason) = ledger.workflow_run_stop_reason(&run.id)? {
                ledger.release_host_lease(&host, &run.id)?;
                return stop(format!(
                    "stop signal before acquiring host '{host}': {reason}"
                ));
            }
            break;
        }
        if lease_started.elapsed() >= lease_wait {
            let holder = ledger.lease_holder(&host)?;
            return fail_pre_exec(format!(
                "host lease '{host}' is held by run {holder:?} after bounded wait"
            ));
        }
        std::thread::sleep(Duration::from_millis(250));
    }
    let _lease = WorkflowLease {
        ledger,
        host: &host,
        run_id: &run.id,
    };
    let mut session = match substrate.acquire(&host, &attempt_dir) {
        Ok(s) => s,
        Err(e) => return fail_pre_exec(format!("acquire: {e:#}")),
    };

    let card = commission(
        doc,
        step,
        snapshot,
        bundle.as_ref().map(|(text, _)| text.as_str()),
    );
    let run_context = serde_json::json!({
        "workflow_run_id": run.id,
        "workflow": run.workflow,
        "revision": run.revision,
        "step": step.name,
        "attempt": attempt,
        "trigger": { "kind": run.trigger_kind, "dedupe_key": run.dedupe_key },
        "agent": snapshot,
        "launch_snapshot_digest": snapshot.digest,
        "authority": &snapshot.authority,
        "authority_digest": snapshot.authority_digest,
        "workflow_grant": &doc.grant,
        "bundle_agents_md_sha256": bundle.as_ref().map(|(_, digest)| digest.clone()),
    })
    .to_string();
    let plan = WorkspacePlan {
        repos: snapshot.repos.clone(),
        card: card.clone(),
        run_context,
        payload: run.payload.clone(),
        report: None,
        pre_command: None,
        post_command: None,
        marker: format!("bb-{step_run_id}"),
        workspace_name: format!("wf-{}-{attempt}", run.id),
        checkpoint: None,
        secrets,
        checkout_secrets: Vec::new(),
        hermetic,
        artifacts: vec![
            OUTCOME_FILENAME.to_string(),
            CHILD_AGENTS_FILENAME.to_string(),
        ],
    };
    if let Err(e) = session.prepare(&plan) {
        let _ = session.release();
        return fail_pre_exec(format!("prepare: {e:#}"));
    }

    let timeout = Duration::from_secs(60 * budget.timeout_minutes.unwrap_or(1));
    // The uniform commission preamble (bitterblossom-971) carries the
    // refused-credential STOP-and-report guardrail; workflow steps are
    // dispatched lanes and receive exactly the same one.
    let prompt = format!("{}\n\n{card}", harness::commission_prompt());
    let max_cost = snapshot.max_cost_per_run_usd.unwrap_or(f64::INFINITY);
    let side_effect_policy = snapshot.side_effect_policy.as_deref().unwrap_or("kill");
    let monitor_needed = max_cost.is_finite() && harness::streams_cost(&snapshot.harness);
    let prior_cost_usd = ledger
        .workflow_run_status(&run.id)?
        .and_then(|status| status.cost_usd)
        .unwrap_or(0.0);
    let mut in_flight = WorkflowInFlightMonitor {
        ledger,
        run_id: &run.id,
        workflow: &run.workflow,
        harness: &snapshot.harness,
        max_cost_usd: max_cost,
        prior_cost_usd,
        side_effect_policy,
        last_recorded_cost: None,
        breached: false,
    };
    let mut monitor_check = |snapshot: &ExecSnapshot<'_>| in_flight.observe(snapshot);
    let mut exec_monitor = ExecMonitor {
        poll_interval: Duration::from_millis(100),
        check: &mut monitor_check,
    };
    let exec = match session.execute(
        &cmd,
        Some(&prompt),
        timeout,
        monitor_needed.then_some(&mut exec_monitor),
    ) {
        Ok(r) => r,
        Err(e) => {
            let _ = session.release();
            let error = format!("execute: {e:#}");
            if e.downcast_ref::<substrate::NoWorkloadStarted>().is_some() {
                return fail_pre_exec(error);
            }
            // Session::execute may fail after its child or remote workload has
            // started. A generic adapter error cannot prove no workload ran,
            // so it is terminal and must never select an ordered fallback.
            return fail_terminal(error, &AttemptStats::default());
        }
    };
    session.write_artifact("stdout.txt", exec.stdout.as_bytes())?;
    session.write_artifact("stderr.txt", exec.stderr.as_bytes())?;
    if let Some(reason) = exec.termination_reason.as_deref() {
        let stats = harness::parse_partial_stats(&agent.harness, &exec.stdout);
        let _ = session.release();
        finish_attempt(
            ledger,
            &run.id,
            &step_run_id,
            "stopped",
            Some(reason),
            Some(exec.exit_code),
            &stats,
        )?;
        return Ok(
            if in_flight.breached && side_effect_policy == "quarantine" {
                StepDisposition::NeedsAttention(reason.to_string())
            } else {
                StepDisposition::Stopped(reason.to_string())
            },
        );
    }
    if exec.timed_out {
        let stats = harness::parse_partial_stats(&agent.harness, &exec.stdout);
        let _ = session.release();
        let reason = format!("wall-clock timeout after {}s (killed)", timeout.as_secs());
        finish_attempt(
            ledger,
            &run.id,
            &step_run_id,
            "failed",
            Some(&reason),
            Some(exec.exit_code),
            &stats,
        )?;
        return Ok(StepDisposition::Failed(reason));
    }
    if exec.exit_code != 0 {
        let stats = harness::parse_partial_stats(&agent.harness, &exec.stdout);
        let _ = session.release();
        let reason = format!(
            "harness exit {}: {}",
            exec.exit_code,
            tail(exec.stderr.trim(), 500)
        );
        finish_attempt(
            ledger,
            &run.id,
            &step_run_id,
            "failed",
            Some(&reason),
            Some(exec.exit_code),
            &stats,
        )?;
        return Ok(StepDisposition::Failed(reason));
    }
    let parsed = match harness::parse_output(&agent.harness, &exec.stdout) {
        Ok(p) => p,
        Err(e) => {
            let stats = harness::parse_partial_stats(&agent.harness, &exec.stdout);
            let _ = session.release();
            let reason = format!("unparseable harness output: {e:#}");
            finish_attempt(
                ledger,
                &run.id,
                &step_run_id,
                "failed",
                Some(&reason),
                Some(exec.exit_code),
                &stats,
            )?;
            return Ok(StepDisposition::Failed(reason));
        }
    };
    session.write_artifact("result.md", parsed.result.as_bytes())?;
    // Release failures remain attempt-scoped, but known spend is retained.
    if let Err(e) = session.release() {
        finish_attempt(
            ledger,
            &run.id,
            &step_run_id,
            "failed",
            Some(&format!("release: {e:#}")),
            Some(exec.exit_code),
            &parsed.stats,
        )?;
        return Ok(StepDisposition::Failed(format!("release: {e:#}")));
    }
    if let Some(cost) = parsed.stats.cost_usd {
        if !cost.is_finite() || cost < 0.0 {
            return fail_stopped(
                format!("harness reported unusable cost_usd {cost}"),
                &parsed.stats,
            );
        }
        ledger.add_workflow_run_cost(&run.id, cost)?;
    }

    if let Some(cost) = parsed.stats.cost_usd {
        if let Some(reason) = final_spend_breach(ledger, run, max_cost, side_effect_policy, cost)? {
            ledger.record_guard_event(
                "workflow_guard_spend_final",
                Some(&run.workflow),
                &reason,
                1,
            )?;
            if side_effect_policy != "log" {
                ledger.finish_workflow_step_run(
                    &step_run_id,
                    "stopped",
                    None,
                    None,
                    Some(&reason),
                    Some(exec.exit_code),
                    &parsed.stats,
                )?;
                return Ok(if side_effect_policy == "quarantine" {
                    StepDisposition::NeedsAttention(reason)
                } else {
                    StepDisposition::Stopped(reason)
                });
            }
        }
    }

    // Child-agent evidence, with monotonic authority narrowing.
    let children_path = attempt_dir.join(CHILD_AGENTS_FILENAME);
    if children_path.exists() {
        let size = std::fs::metadata(&children_path)?.len();
        if size > MAX_CONTRACT_FILE_BYTES {
            return fail_terminal(
                format!("{CHILD_AGENTS_FILENAME} is {size} bytes (max {MAX_CONTRACT_FILE_BYTES})"),
                &parsed.stats,
            );
        }
        let text = std::fs::read_to_string(&children_path)?;
        let decls: Vec<ChildDecl> = match serde_json::from_str(&text) {
            Ok(d) => d,
            Err(e) => {
                return fail_terminal(
                    format!("malformed {CHILD_AGENTS_FILENAME}: {e}"),
                    &parsed.stats,
                )
            }
        };
        for decl in &decls {
            if decl.name.trim().is_empty() {
                return fail_terminal(
                    format!("{CHILD_AGENTS_FILENAME}: child agent name is required"),
                    &parsed.stats,
                );
            }
            if let Some(cost) = decl.cost_usd {
                // Declared spend only ever adds: a negative entry would
                // silently offset siblings' costs inside the enforced sum.
                if cost < 0.0 || !cost.is_finite() {
                    return fail_stopped(
                        format!(
                            "{CHILD_AGENTS_FILENAME}: child agent '{}' declares negative \
                             cost_usd {cost}; declared spend only ever adds",
                            decl.name
                        ),
                        &parsed.stats,
                    );
                }
            }
            let (authority, inherited) = match &decl.authority {
                None => (snapshot.authority.clone(), true),
                Some(declared) => {
                    if let Some(excess) = declared.iter().find(|a| !snapshot.authority.contains(a))
                    {
                        return fail_terminal(
                            format!(
                                "child agent '{}' declares authority {excess:?} beyond its \
                                 parent step grant {:?}; children inherit or narrow, never broaden",
                                decl.name, snapshot.authority
                            ),
                            &parsed.stats,
                        );
                    }
                    (declared.clone(), false)
                }
            };
            ledger.record_workflow_child_agent(
                &step_run_id,
                &decl.name,
                decl.harness.as_deref(),
                decl.model.as_deref(),
                decl.goal.as_deref(),
                &authority,
                inherited,
                decl.cost_usd,
                decl.result.as_deref(),
            )?;
        }
        // Child-declared costs are self-reported evidence, but they are real
        // delegated spend: sum them into the run group's observed cost so
        // the enforced spend cap sees them (conservative — only ever added).
        // A child declaring NO cost stays unmetered evidence; the spend
        // guard's indeterminate rule deliberately reads step attempts only.
        let child_cost: f64 = decls.iter().filter_map(|d| d.cost_usd).sum();
        if child_cost > 0.0 {
            ledger.add_workflow_run_cost(&run.id, child_cost)?;
            if let Some(reason) =
                final_spend_breach(ledger, run, max_cost, side_effect_policy, child_cost)?
            {
                ledger.record_guard_event(
                    "workflow_guard_spend_final",
                    Some(&run.workflow),
                    &reason,
                    1,
                )?;
                if side_effect_policy != "log" {
                    ledger.finish_workflow_step_run(
                        &step_run_id,
                        "stopped",
                        None,
                        None,
                        Some(&reason),
                        Some(exec.exit_code),
                        &parsed.stats,
                    )?;
                    return Ok(if side_effect_policy == "quarantine" {
                        StepDisposition::NeedsAttention(reason)
                    } else {
                        StepDisposition::Stopped(reason)
                    });
                }
            }
        }
    }

    // The completion contract. Branching steps require one declared outcome;
    // the plane never infers one from prose.
    let completion_path = attempt_dir.join(OUTCOME_FILENAME);
    let completion: Option<CompletionDoc> = if completion_path.exists() {
        let size = std::fs::metadata(&completion_path)?.len();
        if size > MAX_CONTRACT_FILE_BYTES {
            let reason = format!(
                "step '{}' attempt {attempt}: {OUTCOME_FILENAME} is {size} bytes \
                 (max {MAX_CONTRACT_FILE_BYTES})",
                step.name
            );
            ledger.finish_workflow_step_run(
                &step_run_id,
                "incomplete",
                None,
                None,
                Some(&reason),
                Some(exec.exit_code),
                &parsed.stats,
            )?;
            return Ok(StepDisposition::Incomplete(reason));
        }
        let text = std::fs::read_to_string(&completion_path)?;
        match serde_json::from_str(&text) {
            Ok(c) => Some(c),
            Err(e) => {
                let reason = format!(
                    "step '{}' attempt {attempt}: malformed {OUTCOME_FILENAME}: {e}",
                    step.name
                );
                ledger.finish_workflow_step_run(
                    &step_run_id,
                    "incomplete",
                    None,
                    None,
                    Some(&reason),
                    Some(exec.exit_code),
                    &parsed.stats,
                )?;
                return Ok(StepDisposition::Incomplete(reason));
            }
        }
    } else {
        None
    };
    let branching = step.routes.len() >= 2;
    let (outcome, incomplete_reason) = if branching {
        let declared: Vec<&str> = step.routes.keys().map(String::as_str).collect();
        match completion.as_ref().and_then(|c| c.outcome.as_deref()) {
            None => (
                None,
                Some(format!(
                    "branching step '{}' declared outcomes [{}] but the agent supplied none in \
                     {OUTCOME_FILENAME}",
                    step.name,
                    declared.join(", ")
                )),
            ),
            Some(supplied) if !step.routes.contains_key(supplied) => (
                None,
                Some(format!(
                    "branching step '{}' received undeclared outcome '{supplied}' \
                     (declared: [{}])",
                    step.name,
                    declared.join(", ")
                )),
            ),
            Some(supplied) => (Some(supplied.to_string()), None),
        }
    } else {
        (completion.as_ref().and_then(|c| c.outcome.clone()), None)
    };
    // Artifact/receipt references stay in the collected OUTCOME.json itself
    // (it is part of the attempt evidence); the row records outcome + summary.
    let summary = completion.as_ref().and_then(|c| c.summary.clone());

    if let Some(reason) = incomplete_reason {
        let reason = format!("step '{}' attempt {attempt}: {reason}", step.name);
        ledger.finish_workflow_step_run(
            &step_run_id,
            "incomplete",
            None,
            summary.as_deref(),
            Some(&reason),
            Some(exec.exit_code),
            &parsed.stats,
        )?;
        return Ok(StepDisposition::Incomplete(reason));
    }
    ledger.finish_workflow_step_run(
        &step_run_id,
        "succeeded",
        outcome.as_deref(),
        summary.as_deref(),
        None,
        Some(exec.exit_code),
        &parsed.stats,
    )?;
    Ok(StepDisposition::Succeeded {
        outcome,
        summary: summary.or_else(|| Some(parsed.result.clone())),
    })
}

// --- views --------------------------------------------------------------------

/// One workflow run's full runtime projection: the immutable acceptance row,
/// the pinned document, the mutable run-group status, and every step attempt
/// with its dynamic children. Same shape for `bb workflow run-show --json`
/// and `GET /api/workflow-runs/<id>`.
pub fn run_detail_view(ledger: &Ledger, run_id: &str) -> Result<serde_json::Value> {
    let run = ledger.workflow_run(run_id)?;
    let mut document: serde_json::Value = serde_json::from_str(
        &ledger
            .workflow_revision(&run.workflow, run.revision)?
            .document,
    )?;
    crate::workflow::project_document_for_readback(&mut document);
    let status = ledger.workflow_run_status(run_id)?;
    let events = ledger.workflow_run_events(run_id)?;
    let workflow = ledger.workflow_by_name(&run.workflow)?;
    let launch_snapshots = ledger.launch_snapshots_for_revision(&workflow.id, run.revision)?;
    let activation = crate::workflow::verified_activation_for_run(ledger, &run)?;
    let steps = ledger
        .workflow_step_runs(run_id)?
        .into_iter()
        .map(|step| {
            let children = ledger.workflow_child_agents(&step.id)?;
            let mut value = serde_json::to_value(&step)?;
            value["children"] = serde_json::to_value(children)?;
            Ok(value)
        })
        .collect::<Result<Vec<_>>>()?;
    Ok(serde_json::json!({
        "run": run,
        "document": document,
        "launch_snapshots": launch_snapshots,
        "activation": activation,
        "status": status,
        "events": events,
        "steps": steps,
    }))
}
