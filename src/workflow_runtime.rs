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

use crate::harness;
use crate::ledger::{new_id, now, AttemptStats, Ledger};
use crate::spec::{AgentSpec, AuthClass, Plane, TaskBudget};
use crate::substrate::{self, WorkspacePlan};
use crate::workflow::{AcceptOutcome, StepAgent, WorkflowDoc, WorkflowStep, ROUTE_DONE};

/// The completion tool: a branching step's agent writes this file with one
/// declared outcome. Single-route steps need no result schema.
pub const OUTCOME_FILENAME: &str = "OUTCOME.json";
/// Dynamic child-agent evidence a step's agent declares for the run tree.
pub const CHILD_AGENTS_FILENAME: &str = "CHILD_AGENTS.json";
const DEFAULT_STEP_TIMEOUT_MINUTES: u64 = 30;
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
pub fn accept(ledger: &Ledger, envelope: &TriggerEnvelope) -> Result<AcceptOutcome> {
    ledger.accept_workflow_run(
        &envelope.workflow,
        envelope.source.kind(),
        envelope.payload.as_deref(),
        envelope.dedupe_key.as_deref(),
    )
}

/// Find the active workflow (if any) declaring a webhook trigger on `route`.
/// Reads the ACTIVE revision document, so pausing or revising a workflow
/// immediately changes what the route accepts.
pub fn webhook_workflow_target(
    ledger: &Ledger,
    route: &str,
) -> Result<Option<(String, crate::workflow::WorkflowTrigger)>> {
    for wf in ledger.list_workflows()? {
        if wf.state != "active" {
            continue;
        }
        let Some(revision) = wf.active_revision else {
            continue;
        };
        // Skip-and-canary: one workflow whose active revision fails to load
        // or validate must never block OTHER workflows' ingress (the hard
        // refusal lives at the execute/rollback doors). Without this, any
        // future validation tightening turns previously-valid active docs
        // into a plane-wide workflow-ingress outage.
        let doc = match load_document(ledger, &wf.name, revision) {
            Ok(doc) => doc,
            Err(e) => {
                skip_unloadable_workflow(&wf.name, &e);
                continue;
            }
        };
        for trigger in &doc.triggers {
            if trigger.kind == "webhook" && trigger.route.as_deref() == Some(route) {
                return Ok(Some((wf.name.clone(), trigger.clone())));
            }
        }
    }
    Ok(None)
}

/// One accepted (or deduplicated) schedule fire, for logs and drills.
#[derive(Debug, Serialize)]
pub struct CronAcceptance {
    pub workflow: String,
    pub scheduled: String,
    pub duplicate: bool,
}

/// Scan active workflows' cron triggers and accept fires due in
/// `(last, now]` through the normalized envelope, deduped on the scheduled
/// timestamp so restarts and overlapping windows never double-accept. At
/// most `max_fires` newest fires per workflow are accepted; skipped older
/// fires are recorded as a guard event, mirroring task cron collapse.
pub fn workflow_cron_tick(
    ledger: &Ledger,
    last_by_workflow: &mut HashMap<String, DateTime<Utc>>,
    default_last: DateTime<Utc>,
    now_utc: DateTime<Utc>,
    max_fires: u32,
) -> Result<Vec<CronAcceptance>> {
    let mut accepted = Vec::new();
    for wf in ledger.list_workflows()? {
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
        for trigger in &doc.triggers {
            if trigger.kind != "cron" {
                continue;
            }
            let schedule =
                crate::ingress::parse_schedule(trigger.schedule.as_deref().unwrap_or(""))
                    .with_context(|| format!("workflow '{}': bad cron trigger", wf.name))?;
            fires.extend(crate::ingress::due_fires(&schedule, last, now_utc));
        }
        fires.sort();
        let max = max_fires.max(1) as usize;
        if fires.len() > max {
            let skipped = fires.len() - max;
            ledger.record_guard_event(
                "workflow_cron_collapse",
                Some(&wf.name),
                &format!("skipped={skipped} fired={}", fires.len()),
                skipped as i64,
            )?;
            fires = fires.split_off(skipped);
        }
        for fire in fires {
            let scheduled = fire.to_rfc3339();
            let outcome = accept(
                ledger,
                &TriggerEnvelope {
                    workflow: wf.name.clone(),
                    source: TriggerSource::Schedule,
                    payload: None,
                    dedupe_key: Some(format!("cron:{scheduled}")),
                },
            )?;
            accepted.push(CronAcceptance {
                workflow: wf.name.clone(),
                scheduled,
                duplicate: matches!(outcome, AcceptOutcome::Duplicate { .. }),
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

#[derive(Debug, Serialize)]
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

    /// Claim a queued run for execution: queued -> running CAS, so a serve
    /// runner and a CLI `bb workflow execute` can never both run one group.
    /// Store-era runs accepted before this table existed get a status row on
    /// first claim.
    pub fn claim_workflow_run(&self, run_id: &str) -> Result<bool> {
        let ts = now();
        self.conn.execute(
            "INSERT OR IGNORE INTO workflow_run_status (run_id, state, updated_at)
             VALUES (?1, 'queued', ?2)",
            params![run_id, ts],
        )?;
        let updated = self.conn.execute(
            "UPDATE workflow_run_status
             SET state = 'running', started_at = ?2, updated_at = ?2
             WHERE run_id = ?1 AND state = 'queued'",
            params![run_id, ts],
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
        let ts = now();
        self.conn.execute(
            "INSERT OR IGNORE INTO workflow_run_status (run_id, state, updated_at)
             VALUES (?1, 'queued', ?2)",
            params![run_id, ts],
        )?;
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

    pub fn queued_workflow_run_ids(&self) -> Result<Vec<String>> {
        let mut stmt = self.conn.prepare(
            "SELECT run_id FROM workflow_run_status WHERE state = 'queued' ORDER BY updated_at, run_id",
        )?;
        let ids = stmt
            .query_map([], |r| r.get(0))?
            .collect::<rusqlite::Result<Vec<String>>>()?;
        Ok(ids)
    }

    fn running_workflow_run_ids(&self) -> Result<Vec<String>> {
        let mut stmt = self
            .conn
            .prepare("SELECT run_id FROM workflow_run_status WHERE state = 'running'")?;
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
                stats.cost_usd,
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
    pub disposition: String,
}

/// Classify workflow runs inherited in `running` state at boot. A step
/// attempt may have external side effects, so the plane never blindly
/// re-executes: the run becomes `needs_attention` naming the exact
/// uncertainty, and replay is an explicit operator act.
pub fn recover_inherited_workflow_runs(ledger: &Ledger) -> Result<Vec<WorkflowRecoveryReport>> {
    let mut reports = Vec::new();
    for run_id in ledger.running_workflow_run_ids()? {
        let run = ledger.workflow_run(&run_id)?;
        let status = ledger
            .workflow_run_status(&run_id)?
            .context("running workflow run has no status row")?;
        let step = status.current_step.clone();
        let detail = format!(
            "inherited running run at boot; step '{}' was in flight when the plane stopped — \
             side effects unknown, not re-executed; inspect step artifacts, then accept a new \
             event explicitly if the work must be redone",
            step.as_deref().unwrap_or("-")
        );
        ledger.set_workflow_run_state(&run_id, "needs_attention", Some(&detail))?;
        reports.push(WorkflowRecoveryReport {
            run_id,
            workflow: run.workflow,
            step,
            disposition: "needs_attention".to_string(),
        });
    }
    Ok(reports)
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
    Failed(String),
    Incomplete(String),
}

/// Execute one accepted workflow run to a terminal state. Claims the run
/// (queued -> running CAS); refuses anything already claimed or terminal.
pub fn execute_run(plane: &Plane, ledger: &Ledger, run_id: &str) -> Result<serde_json::Value> {
    match execute_if_queued(plane, ledger, run_id)? {
        Some(view) => Ok(view),
        None => {
            let state = ledger
                .workflow_run_status(run_id)?
                .map(|s| s.state)
                .unwrap_or_else(|| "unknown".to_string());
            bail!("workflow run {run_id} is {state}; only queued runs execute");
        }
    }
}

/// Runner-loop variant of [`execute_run`]: returns `Ok(None)` when the
/// queued->running claim was lost (someone else is executing), which the
/// serve loop treats as a non-event instead of an error.
pub fn execute_if_queued(
    plane: &Plane,
    ledger: &Ledger,
    run_id: &str,
) -> Result<Option<serde_json::Value>> {
    let run = ledger.workflow_run(run_id)?;
    if !ledger.claim_workflow_run(run_id)? {
        return Ok(None);
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
        Ok(view) => Ok(Some(view)),
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
    let doc = load_document(ledger, &run.workflow, run.revision)?;
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

        // Guards, checked before every attempt; first fired wins.
        if let Some(guard) = fired_guard(ledger, run, &doc, step, started)? {
            break ("stopped", Some(guard.clone()), "workflow_run_stopped");
        }

        match run_step(plane, ledger, run, &doc, step)? {
            StepDisposition::Failed(error) => {
                break ("failed", Some(error), "workflow_run_failed");
            }
            StepDisposition::Incomplete(reason) => {
                break ("incomplete", Some(reason), "workflow_run_incomplete");
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
fn fired_guard(
    ledger: &Ledger,
    run: &crate::workflow::WorkflowRunRow,
    doc: &WorkflowDoc,
    step: &WorkflowStep,
    started: Instant,
) -> Result<Option<String>> {
    let fired = if let Some(reason) = ledger.workflow_run_stop_reason(&run.id)? {
        Some((
            "workflow_guard_stop_signal",
            format!("stop signal: {reason}"),
        ))
    } else if let Some(max) = doc.policies.max_rounds {
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
            if let Some(max) = doc.policies.max_elapsed_seconds {
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
            if let Some(cap) = doc.policies.max_cost_per_run_usd {
                // When spend is the ONLY cycle guard, an attempt that
                // reported no cost makes the guard indeterminate: unknown
                // spend is never treated as zero (the status-row contract),
                // so the cycle stops naming exactly that instead of looping
                // unmetered. Bounded runs (rounds/elapsed declared, or no
                // cycle at all) proceed; the cap simply cannot see the
                // unmetered attempts.
                let spend_is_only_cycle_guard =
                    doc.policies.max_rounds.is_none() && doc.policies.max_elapsed_seconds.is_none();
                // Scoped to steps ON cycles: a validation-admitted blind
                // entry step off the cycle runs a bounded number of times
                // and must not make every run of the shape dead on arrival.
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
                if unmetered > 0 {
                    Some((
                        "workflow_guard_spend_indeterminate",
                        format!(
                            "spend guard indeterminate: {unmetered} cycle-step attempt(s) \
                             reported no cost and max_cost_per_run_usd is the only cycle guard \
                             — unknown spend is never treated as zero"
                        ),
                    ))
                } else {
                    let observed = ledger
                        .workflow_run_status(&run.id)?
                        .and_then(|s| s.cost_usd)
                        .unwrap_or(0.0);
                    (observed > cap).then(|| {
                        (
                            "workflow_guard_spend",
                            format!(
                                "spend guard: observed ${observed:.4} of run-group cap ${cap:.2}"
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

fn agent_spec_from(agent: &StepAgent) -> AgentSpec {
    AgentSpec {
        version: agent.version,
        harness: agent.harness.clone(),
        model: agent.model.clone(),
        role: None,
        skills: Vec::new(),
        provider: agent.provider.clone(),
        auth: None,
        bin: agent.bin.clone(),
        args: agent.args.clone(),
        secrets: agent.secrets.clone(),
        checkout_secrets: Vec::new(),
        optional_secrets: Vec::new(),
        policy: Default::default(),
        roster: None,
    }
}

/// Roster v0.2 resolved bundle consumption: the bundle's AGENTS.md joins the
/// commission and its digest is recorded as provenance. A declared-but-
/// missing bundle fails honestly instead of launching a different agent.
fn load_bundle(agent: &StepAgent) -> Result<Option<(String, String)>> {
    let Some(bundle) = &agent.bundle else {
        return Ok(None);
    };
    let path = PathBuf::from(bundle).join("AGENTS.md");
    let text = std::fs::read_to_string(&path)
        .with_context(|| format!("roster bundle AGENTS.md at {}", path.display()))?;
    use sha2::{Digest, Sha256};
    let digest = format!("{:x}", Sha256::digest(text.as_bytes()));
    Ok(Some((text, digest)))
}

/// The step commission: workflow + step goals plus the mechanical completion
/// contract projected from config (declared outcomes, authority labels).
/// Projection, not judgment — every semantic token here comes from the
/// pinned document.
fn commission(doc: &WorkflowDoc, step: &WorkflowStep, bundle_agents_md: Option<&str>) -> String {
    let mut card = String::new();
    if let Some(agents_md) = bundle_agents_md {
        card.push_str(agents_md.trim_end());
        card.push_str("\n\n---\n\n");
    }
    card.push_str(&format!(
        "# Workflow step commission\n\nWorkflow: {} — {}\nStep: {}\n\n{}\n",
        doc.name, doc.goal, step.name, step.goal
    ));
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
        step.authority
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

fn run_step(
    plane: &Plane,
    ledger: &Ledger,
    run: &crate::workflow::WorkflowRunRow,
    doc: &WorkflowDoc,
    step: &WorkflowStep,
) -> Result<StepDisposition> {
    let attempt = ledger.next_workflow_attempt(&run.id)?;
    let attempt_dir = plane
        .root
        .join(".bb/workflow-runs")
        .join(&run.id)
        .join(format!("attempt-{attempt}-{}", step.name));
    std::fs::create_dir_all(&attempt_dir)?;
    let agent_json = serde_json::to_string(&step.agent)?;
    let step_run_id = ledger.create_workflow_step_run(
        &run.id,
        &step.name,
        attempt,
        &agent_json,
        &step.goal,
        &step.authority,
        &attempt_dir.to_string_lossy(),
    )?;
    ledger.set_workflow_run_current_step(&run.id, &step.name)?;

    let fail = |error: String, stats: &AttemptStats| -> Result<StepDisposition> {
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
    let none = AttemptStats::default();

    let bundle = match load_bundle(&step.agent) {
        Ok(b) => b,
        Err(e) => return fail(format!("{e:#}"), &none),
    };
    let agent = agent_spec_from(&step.agent);
    let hermetic = match agent.auth_class() {
        Ok(AuthClass::Api) => true,
        Ok(AuthClass::Subscription) => false,
        Err(e) => return fail(format!("{e:#}"), &none),
    };
    let budget = TaskBudget {
        timeout_minutes: Some(
            doc.policies
                .timeout_minutes
                .unwrap_or(DEFAULT_STEP_TIMEOUT_MINUTES),
        ),
        ..Default::default()
    };
    let cmd = match harness::build_command(&agent, &budget) {
        Ok(c) => c,
        Err(e) => return fail(format!("build_command: {e:#}"), &none),
    };
    let mut secrets = Vec::new();
    for name in &step.agent.secrets {
        match std::env::var(name) {
            Ok(value) => secrets.push((name.clone(), value)),
            Err(_) => return fail(format!("secret env var '{name}' not set"), &none),
        }
    }

    let substrate_kind = doc.policies.substrate.as_deref().unwrap_or("local");
    if substrate_kind == "local" && !plane.spec.dev {
        // Same posture as task-land: local exec is dev/test machinery.
        return fail(
            "substrate 'local' requires a dev plane (`dev = true` in plane.toml); \
             declare policies.substrate for production execution"
                .to_string(),
            &none,
        );
    }
    let substrate = match substrate::for_task(substrate_kind) {
        Ok(s) => s,
        Err(e) => return fail(format!("{e:#}"), &none),
    };
    let mut session = match substrate.acquire(&format!("wf-{}", run.id), &attempt_dir) {
        Ok(s) => s,
        Err(e) => return fail(format!("acquire: {e:#}"), &none),
    };

    let card = commission(doc, step, bundle.as_ref().map(|(text, _)| text.as_str()));
    let run_context = serde_json::json!({
        "workflow_run_id": run.id,
        "workflow": run.workflow,
        "revision": run.revision,
        "step": step.name,
        "attempt": attempt,
        "trigger": { "kind": run.trigger_kind, "dedupe_key": run.dedupe_key },
        "agent": &step.agent,
        "authority": &step.authority,
        "bundle_agents_md_sha256": bundle.as_ref().map(|(_, digest)| digest.clone()),
    })
    .to_string();
    let plan = WorkspacePlan {
        repos: Vec::new(),
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
        return fail(format!("prepare: {e:#}"), &none);
    }

    let timeout = Duration::from_secs(60 * budget.timeout_minutes.unwrap_or(1));
    // The uniform commission preamble (bitterblossom-971) carries the
    // refused-credential STOP-and-report guardrail; workflow steps are
    // dispatched lanes and receive exactly the same one.
    let prompt = format!("{}\n\n{card}", harness::commission_prompt());
    let exec = match session.execute(&cmd, Some(&prompt), timeout, None) {
        Ok(r) => r,
        Err(e) => {
            let _ = session.release();
            return fail(format!("execute: {e:#}"), &none);
        }
    };
    session.write_artifact("stdout.txt", exec.stdout.as_bytes())?;
    session.write_artifact("stderr.txt", exec.stderr.as_bytes())?;
    if exec.timed_out {
        let _ = session.release();
        return fail(
            format!("wall-clock timeout after {}s (killed)", timeout.as_secs()),
            &none,
        );
    }
    if exec.exit_code != 0 {
        let _ = session.release();
        return fail(
            format!(
                "harness exit {}: {}",
                exec.exit_code,
                tail(exec.stderr.trim(), 500)
            ),
            &none,
        );
    }
    let parsed = match harness::parse_output(&agent.harness, &exec.stdout) {
        Ok(p) => p,
        Err(e) => {
            let _ = session.release();
            return fail(format!("unparseable harness output: {e:#}"), &none);
        }
    };
    session.write_artifact("result.md", parsed.result.as_bytes())?;
    // Release failures (including the substrate's own artifact size cap) are
    // attempt-scoped: a named step failure, never an executor error.
    if let Err(e) = session.release() {
        return fail(format!("release: {e:#}"), &parsed.stats);
    }
    if let Some(cost) = parsed.stats.cost_usd {
        ledger.add_workflow_run_cost(&run.id, cost)?;
    }

    // Child-agent evidence, with monotonic authority narrowing.
    let children_path = attempt_dir.join(CHILD_AGENTS_FILENAME);
    if children_path.exists() {
        let size = std::fs::metadata(&children_path)?.len();
        if size > MAX_CONTRACT_FILE_BYTES {
            return fail(
                format!("{CHILD_AGENTS_FILENAME} is {size} bytes (max {MAX_CONTRACT_FILE_BYTES})"),
                &parsed.stats,
            );
        }
        let text = std::fs::read_to_string(&children_path)?;
        let decls: Vec<ChildDecl> = match serde_json::from_str(&text) {
            Ok(d) => d,
            Err(e) => {
                return fail(
                    format!("malformed {CHILD_AGENTS_FILENAME}: {e}"),
                    &parsed.stats,
                )
            }
        };
        for decl in &decls {
            if decl.name.trim().is_empty() {
                return fail(
                    format!("{CHILD_AGENTS_FILENAME}: child agent name is required"),
                    &parsed.stats,
                );
            }
            if let Some(cost) = decl.cost_usd {
                // Declared spend only ever adds: a negative entry would
                // silently offset siblings' costs inside the enforced sum.
                if cost < 0.0 || cost.is_nan() {
                    return fail(
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
                None => (step.authority.clone(), true),
                Some(declared) => {
                    if let Some(excess) = declared.iter().find(|a| !step.authority.contains(a)) {
                        return fail(
                            format!(
                                "child agent '{}' declares authority {excess:?} beyond its \
                                 parent step grant {:?}; children inherit or narrow, never broaden",
                                decl.name, step.authority
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
    let document: serde_json::Value = serde_json::from_str(
        &ledger
            .workflow_revision(&run.workflow, run.revision)?
            .document,
    )?;
    let status = ledger.workflow_run_status(run_id)?;
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
        "status": status,
        "steps": steps,
    }))
}
