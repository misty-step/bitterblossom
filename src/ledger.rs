use std::path::Path;

use anyhow::{bail, Context, Result};
use rusqlite::{params, Connection, OptionalExtension};
use serde::Serialize;
use time::format_description::well_known::Rfc3339;
use time::OffsetDateTime;

pub fn now() -> String {
    OffsetDateTime::now_utc()
        .format(&Rfc3339)
        .expect("rfc3339 format")
}
pub const RUN_STATES: &[&str] = &[
    "pending",
    "running",
    "success",
    "failure",
    "awaiting_recovery",
    "blocked_budget",
    "retired",
];

fn transition_allowed(from: &str, to: &str) -> bool {
    matches!(
        (from, to),
        ("pending", "running")
            | ("pending", "failure")
            | ("pending", "blocked_budget")
            | ("running", "success")
            | ("running", "failure")
            | ("running", "awaiting_recovery")
            | ("blocked_budget", "pending")
            | ("blocked_budget", "retired")
            | ("awaiting_recovery", "success")
            | ("awaiting_recovery", "failure")
    )
}
pub const ATTEMPT_PHASES: &[&str] = &[
    "acquired",
    "prepared",
    "executing",
    "collecting",
    "finalizing",
    "released",
];

pub fn phase_reached(phase: &str, milestone: &str) -> bool {
    let idx = |p| ATTEMPT_PHASES.iter().position(|&x| x == p);
    match (idx(phase), idx(milestone)) {
        (Some(a), Some(b)) => a >= b,
        _ => false,
    }
}

#[derive(Debug, Serialize)]
pub struct RunRow {
    pub id: String,
    pub task: String,
    pub trigger_kind: String,
    pub idempotency_key: Option<String>,
    pub state: String,
    pub state_reason: Option<String>,
    pub trace_id: String,
    pub parent_run_id: Option<String>,
    pub agent_name: Option<String>,
    pub agent_version: Option<i64>,
    pub config_source_repo: Option<String>,
    pub config_source_ref: Option<String>,
    pub cost_usd: Option<f64>,
    pub duration_ms: Option<i64>,
    pub created_at: String,
    pub updated_at: String,
}

#[derive(Debug, Serialize)]
pub struct AttemptRow {
    pub id: i64,
    pub run_id: String,
    pub n: i64,
    pub agent_name: String,
    pub agent_version: i64,
    pub harness: String,
    pub model: String,
    pub phase: String,
    pub outcome: Option<String>,
    pub error: Option<String>,
    pub exit_code: Option<i64>,
    pub tokens_in: Option<i64>,
    pub tokens_out: Option<i64>,
    pub turns: Option<i64>,
    pub cost_usd: Option<f64>,
    pub artifact_dir: Option<String>,
    pub started_at: String,
    pub ended_at: Option<String>,
}

#[derive(Serialize)]
pub struct RunEventRow {
    pub run_id: String,
    pub kind: String,
    pub data: Option<String>,
    pub at: String,
}

#[derive(Serialize)]
pub struct DeadLetterRow {
    pub id: i64,
    pub run_id: String,
    pub task: String,
    pub payload: Option<String>,
    pub error: String,
    pub created_at: String,
    pub replayed_run_id: Option<String>,
    pub acknowledged_reason: Option<String>,
    pub acknowledged_at: Option<String>,
    /// `open` (no replay, no acknowledgement), `replayed`, or `acknowledged`.
    pub status: String,
}

/// Resolution state of a dead letter, derived from its replay/acknowledgement
/// columns. Acknowledgement and replay are mutually exclusive operator paths
/// to close a pre-execute dead letter; replay history is immutable.
pub fn dlq_status(replayed: Option<&str>, acknowledged_at: Option<&str>) -> String {
    if replayed.is_some() {
        "replayed".into()
    } else if acknowledged_at.is_some() {
        "acknowledged".into()
    } else {
        "open".into()
    }
}

pub struct Ledger {
    pub(crate) conn: Connection,
}

const SCHEMA: &str = include_str!("schema.sql");

impl Ledger {
    pub fn open(path: &Path) -> Result<Self> {
        if let Some(parent) = path.parent() {
            std::fs::create_dir_all(parent)?;
        }
        let conn =
            Connection::open(path).with_context(|| format!("open ledger {}", path.display()))?;
        conn.pragma_update(None, "journal_mode", "WAL")?;
        conn.pragma_update(None, "busy_timeout", 5000_i64)?;
        conn.pragma_update(None, "foreign_keys", "ON")?;
        conn.execute_batch(SCHEMA)?;
        ensure_column(&conn, "runs", "config_source_repo", "TEXT")?;
        ensure_column(&conn, "runs", "config_source_ref", "TEXT")?;
        ensure_column(&conn, "dead_letters", "acknowledged_reason", "TEXT")?;
        ensure_column(&conn, "dead_letters", "acknowledged_at", "TEXT")?;
        Ok(Self { conn })
    }

    pub fn ingest(&mut self, req: IngressRequest<'_>) -> Result<IngressOutcome> {
        let tx = self
            .conn
            .transaction_with_behavior(rusqlite::TransactionBehavior::Immediate)?;
        let ts = now();

        let candidate_id = new_id();
        let trace_id = new_id();
        let parked: Option<String> = tx
            .query_row(
                "SELECT reason FROM parked_tasks WHERE task = ?1",
                params![req.task],
                |r| r.get(0),
            )
            .optional()?;
        let (state, reason) = match &parked {
            Some(reason) => ("blocked_budget", Some(format!("task parked: {reason}"))),
            None => ("pending", None),
        };
        let inserted = tx.execute(
            "INSERT INTO runs (id, task, trigger_kind, idempotency_key, state,
               state_reason, trace_id, parent_run_id, payload, created_at, updated_at)
             VALUES (?1, ?2, ?3, ?4, ?5, ?6, ?7, ?8, ?9, ?10, ?10)
             ON CONFLICT(task, idempotency_key) WHERE idempotency_key IS NOT NULL
             DO NOTHING",
            params![
                candidate_id,
                req.task,
                req.trigger_kind,
                req.idempotency_key,
                state,
                reason,
                trace_id,
                req.parent_run_id,
                req.payload,
                ts
            ],
        )?;
        let (run_id, duplicate) = if inserted == 1 {
            (candidate_id, false)
        } else {
            let key = req.idempotency_key.expect("conflict implies a key");
            let existing: String = tx.query_row(
                "SELECT id FROM runs WHERE task = ?1 AND idempotency_key = ?2",
                params![req.task, key],
                |r| r.get(0),
            )?;
            (existing, true)
        };

        tx.execute(
            "INSERT INTO ingress_events (run_id, task, trigger_kind, source_event_id,
               dedupe_key, payload_hash, duplicate, received_at)
             VALUES (?1, ?2, ?3, ?4, ?5, ?6, ?7, ?8)",
            params![
                run_id,
                req.task,
                req.trigger_kind,
                req.source_event_id,
                req.idempotency_key,
                req.payload.map(payload_hash),
                duplicate as i64,
                ts
            ],
        )?;
        tx.commit()?;

        let state = self.run_state(&run_id)?;
        Ok(IngressOutcome {
            run_id,
            duplicate,
            state,
        })
    }

    pub fn run_state(&self, run_id: &str) -> Result<String> {
        self.conn
            .query_row(
                "SELECT state FROM runs WHERE id = ?1",
                params![run_id],
                |r| r.get(0),
            )
            .with_context(|| format!("run {run_id} not found"))
    }
    pub fn try_transition(&self, run_id: &str, to: &str, reason: Option<&str>) -> Result<bool> {
        let sources: Vec<&str> = RUN_STATES
            .iter()
            .copied()
            .filter(|from| transition_allowed(from, to))
            .collect();
        if sources.is_empty() {
            bail!("no legal transition into state '{to}'");
        }
        let list: Vec<String> = sources.iter().map(|s| format!("'{s}'")).collect();
        let sql = format!(
            "UPDATE runs SET state = ?2, state_reason = ?3, updated_at = ?4
             WHERE id = ?1 AND state IN ({})",
            list.join(", "),
        );
        let changed = self
            .conn
            .execute(&sql, params![run_id, to, reason, now()])?;
        if changed == 1 {
            self.record_event(run_id, &format!("state:{to}"), reason)?;
        }
        Ok(changed == 1)
    }
    pub fn transition(&self, run_id: &str, to: &str, reason: Option<&str>) -> Result<()> {
        if !self.try_transition(run_id, to, reason)? {
            let from = self.run_state(run_id)?;
            bail!("illegal run transition {from} -> {to} for {run_id}");
        }
        Ok(())
    }

    pub fn finalize_run(
        &self,
        run_id: &str,
        cost_usd: Option<f64>,
        duration_ms: i64,
    ) -> Result<()> {
        self.conn.execute(
            "UPDATE runs SET cost_usd = ?2, duration_ms = ?3, updated_at = ?4 WHERE id = ?1",
            params![run_id, cost_usd, duration_ms, now()],
        )?;
        Ok(())
    }

    pub fn set_run_agent(&self, run_id: &str, agent_name: &str, agent_version: u32) -> Result<()> {
        self.conn.execute(
            "UPDATE runs SET agent_name = ?2, agent_version = ?3, updated_at = ?4 WHERE id = ?1",
            params![run_id, agent_name, agent_version as i64, now()],
        )?;
        Ok(())
    }

    pub fn set_run_config_source(&self, run_id: &str, repo: &str, ref_: &str) -> Result<()> {
        self.conn.execute(
            "UPDATE runs SET config_source_repo = ?2, config_source_ref = ?3,
               updated_at = ?4 WHERE id = ?1",
            params![run_id, repo, ref_, now()],
        )?;
        Ok(())
    }

    pub fn record_event(&self, run_id: &str, kind: &str, data: Option<&str>) -> Result<()> {
        self.conn.execute(
            "INSERT INTO run_events (run_id, kind, data, at) VALUES (?1, ?2, ?3, ?4)",
            params![run_id, kind, data, now()],
        )?;
        Ok(())
    }

    pub fn create_attempt(
        &self,
        run_id: &str,
        n: i64,
        agent_name: &str,
        agent_version: u32,
        harness: &str,
        model: &str,
    ) -> Result<i64> {
        self.conn.execute(
            "INSERT INTO attempts (run_id, n, agent_name, agent_version, harness, model,
               phase, started_at)
             VALUES (?1, ?2, ?3, ?4, ?5, ?6, 'acquired', ?7)",
            params![
                run_id,
                n,
                agent_name,
                agent_version as i64,
                harness,
                model,
                now()
            ],
        )?;
        Ok(self.conn.last_insert_rowid())
    }

    pub fn set_attempt_phase(&self, attempt_id: i64, phase: &str) -> Result<()> {
        if !ATTEMPT_PHASES.contains(&phase) {
            bail!("unknown attempt phase {phase}");
        }
        self.conn.execute(
            "UPDATE attempts SET phase = ?2 WHERE id = ?1",
            params![attempt_id, phase],
        )?;
        Ok(())
    }

    pub fn attempt_phase(&self, attempt_id: i64) -> Result<String> {
        Ok(self.conn.query_row(
            "SELECT phase FROM attempts WHERE id = ?1",
            params![attempt_id],
            |r| r.get(0),
        )?)
    }

    #[allow(clippy::too_many_arguments)]
    pub fn finish_attempt(
        &self,
        attempt_id: i64,
        outcome: &str,
        error: Option<&str>,
        exit_code: Option<i64>,
        stats: &AttemptStats,
        artifact_dir: Option<&str>,
    ) -> Result<()> {
        self.conn.execute(
            "UPDATE attempts SET outcome = ?2, error = ?3, exit_code = ?4, tokens_in = ?5,
               tokens_out = ?6, turns = ?7, cost_usd = ?8, artifact_dir = ?9, ended_at = ?10
             WHERE id = ?1",
            params![
                attempt_id,
                outcome,
                error,
                exit_code,
                stats.tokens_in,
                stats.tokens_out,
                stats.turns,
                stats.cost_usd,
                artifact_dir,
                now()
            ],
        )?;
        Ok(())
    }

    pub fn attempt_count(&self, run_id: &str) -> Result<i64> {
        Ok(self.conn.query_row(
            "SELECT COUNT(*) FROM attempts WHERE run_id = ?1",
            params![run_id],
            |r| r.get(0),
        )?)
    }
    pub fn try_acquire_host_lease(&self, host: &str, run_id: &str) -> Result<bool> {
        let n = self.conn.execute(
            "INSERT INTO host_leases (host, run_id, acquired_at) VALUES (?1, ?2, ?3)
             ON CONFLICT(host) DO NOTHING",
            params![host, run_id, now()],
        )?;
        Ok(n == 1)
    }

    pub fn release_host_lease(&self, host: &str, run_id: &str) -> Result<()> {
        self.conn.execute(
            "DELETE FROM host_leases WHERE host = ?1 AND run_id = ?2",
            params![host, run_id],
        )?;
        Ok(())
    }

    pub fn lease_holder(&self, host: &str) -> Result<Option<String>> {
        Ok(self
            .conn
            .query_row(
                "SELECT run_id FROM host_leases WHERE host = ?1",
                params![host],
                |r| r.get(0),
            )
            .optional()?)
    }

    pub fn record_dead_letter(
        &self,
        run_id: &str,
        task: &str,
        payload: Option<&str>,
        error: &str,
    ) -> Result<i64> {
        self.conn.execute(
            "INSERT INTO dead_letters (run_id, task, payload, error, created_at)
             VALUES (?1, ?2, ?3, ?4, ?5)",
            params![run_id, task, payload, error, now()],
        )?;
        Ok(self.conn.last_insert_rowid())
    }

    pub fn dead_letter(&self, id: i64) -> Result<DeadLetterRow> {
        self.conn
            .query_row(
                &format!("{DLQ_SELECT} WHERE id = ?1"),
                params![id],
                row_to_dlq,
            )
            .with_context(|| format!("dead letter {id} not found"))
    }

    pub fn list_dead_letters(&self) -> Result<Vec<DeadLetterRow>> {
        let mut stmt = self
            .conn
            .prepare(&format!("{DLQ_SELECT} ORDER BY id DESC"))?;
        let rows = stmt
            .query_map([], row_to_dlq)?
            .collect::<rusqlite::Result<Vec<_>>>()?;
        Ok(rows)
    }
    pub fn mark_dead_letter_replayed(&self, id: i64, new_run_id: &str) -> Result<bool> {
        let changed = self.conn.execute(
            "UPDATE dead_letters SET replayed_run_id = ?2
             WHERE id = ?1
               AND acknowledged_at IS NULL
               AND (replayed_run_id IS NULL OR replayed_run_id = ?2)",
            params![id, new_run_id],
        )?;
        Ok(changed == 1)
    }
    pub fn release_leases_for_run(&self, run_id: &str) -> Result<()> {
        self.conn
            .execute("DELETE FROM host_leases WHERE run_id = ?1", params![run_id])?;
        Ok(())
    }

    pub fn record_budget_event(&self, task: Option<&str>, kind: &str, detail: &str) -> Result<()> {
        self.conn.execute(
            "INSERT INTO budget_events (task, kind, detail, at) VALUES (?1, ?2, ?3, ?4)",
            params![task, kind, detail, now()],
        )?;
        Ok(())
    }

    pub fn park_task(&self, task: &str, reason: &str) -> Result<()> {
        self.conn.execute(
            "INSERT INTO parked_tasks (task, reason, at) VALUES (?1, ?2, ?3)
             ON CONFLICT(task) DO UPDATE SET reason = ?2, at = ?3",
            params![task, reason, now()],
        )?;
        Ok(())
    }
    /// Acknowledge a pre-execute dead letter as superseded without replaying
    /// it. Requires a non-empty operator reason. Refuses a dead letter that is
    /// already replayed (replay is the other resolution path) or already
    /// acknowledged. Replay history (`replayed_run_id`) is never mutated here.
    /// Records a `dlq:acknowledged` event on the originating run for traceability.
    pub fn acknowledge_dead_letter(&self, id: i64, reason: &str) -> Result<DeadLetterRow> {
        let dl = self.dead_letter(id)?;
        if let Some(prev) = &dl.replayed_run_id {
            bail!("dead letter {id} already replayed as run {prev}; acknowledge is for unreplayed rows");
        }
        if let Some(prev_reason) = &dl.acknowledged_reason {
            bail!("dead letter {id} already acknowledged: {prev_reason}");
        }
        let reason = reason.trim();
        if reason.is_empty() {
            bail!("acknowledging dead letter {id} requires a non-empty reason");
        }
        let changed = self.conn.execute(
            "UPDATE dead_letters SET acknowledged_reason = ?2, acknowledged_at = ?3
             WHERE id = ?1 AND replayed_run_id IS NULL AND acknowledged_at IS NULL",
            params![id, reason, now()],
        )?;
        if changed != 1 {
            let current = self.dead_letter(id)?;
            bail!(
                "dead letter {id} is {}; acknowledge requires an open row",
                current.status
            );
        }
        self.record_event(&dl.run_id, "dlq:acknowledged", Some(reason))?;
        self.dead_letter(id)
    }

    pub fn unpark_task(&self, task: &str) -> Result<Vec<String>> {
        self.conn
            .execute("DELETE FROM parked_tasks WHERE task = ?1", params![task])?;
        let mut stmt = self
            .conn
            .prepare("SELECT id FROM runs WHERE task = ?1 AND state = 'blocked_budget'")?;
        let blocked: Vec<String> = stmt
            .query_map(params![task], |r| r.get(0))?
            .collect::<rusqlite::Result<Vec<_>>>()?;
        for run_id in &blocked {
            self.transition(run_id, "pending", Some("unparked"))?;
        }
        Ok(blocked)
    }

    pub fn parked_reason(&self, task: &str) -> Result<Option<String>> {
        Ok(self
            .conn
            .query_row(
                "SELECT reason FROM parked_tasks WHERE task = ?1",
                params![task],
                |r| r.get(0),
            )
            .optional()?)
    }
    /// Re-queue one budget-blocked run back to `pending` and clear its task's
    /// park so it can dispatch. Other blocked runs for the task are left as-is
    /// (contrast `unpark_task`, which releases the whole parked queue). Refuses a
    /// run held by a budget limit with no park (e.g. the global daily cost
    /// ceiling): there is nothing to clear and it would just re-block — close it
    /// with `retire` or wait for the limit to reset. A re-queued run can still
    /// re-block if another limit (e.g. `max_runs_per_day`) is still over.
    pub fn release_blocked_run(&self, run_id: &str, reason: &str) -> Result<()> {
        let run = self.require_blocked_budget(run_id)?;
        if self.parked_reason(&run.task)?.is_none() {
            bail!(
                "run {run_id} is held by a budget limit on task '{}', not a park; \
                 release cannot clear it (retire it, or wait for the limit to reset)",
                run.task
            );
        }
        self.conn.execute(
            "DELETE FROM parked_tasks WHERE task = ?1",
            params![run.task],
        )?;
        self.transition(run_id, "pending", Some(reason))?;
        self.record_budget_event(Some(&run.task), "run_released", reason)
    }

    /// Retire one budget-blocked run as intentionally not-to-run, keeping the
    /// ledger row and its history. Leaves the task's park untouched.
    pub fn retire_blocked_run(&self, run_id: &str, reason: &str) -> Result<()> {
        let run = self.require_blocked_budget(run_id)?;
        self.transition(run_id, "retired", Some(reason))?;
        self.record_budget_event(Some(&run.task), "run_retired", reason)
    }

    fn require_blocked_budget(&self, run_id: &str) -> Result<RunRow> {
        let run = self.run(run_id)?;
        if run.state != "blocked_budget" {
            bail!("run {run_id} is '{}', not 'blocked_budget'", run.state);
        }
        Ok(run)
    }

    pub fn runs_today(&self, task: &str) -> Result<i64> {
        let day = &now()[..10];
        Ok(self.conn.query_row(
            "SELECT COUNT(*) FROM runs WHERE task = ?1
             AND state NOT IN ('blocked_budget', 'pending', 'retired')
             AND substr(created_at, 1, 10) = ?2",
            params![task, day],
            |r| r.get(0),
        )?)
    }
    pub fn cost_today(&self) -> Result<f64> {
        let day = &now()[..10];
        Ok(self.conn.query_row(
            "SELECT COALESCE(SUM(cost_usd), 0.0) FROM attempts WHERE substr(started_at, 1, 10) = ?1",
            params![day],
            |r| r.get(0),
        )?)
    }

    pub fn run(&self, run_id: &str) -> Result<RunRow> {
        self.conn
            .query_row(
                &format!("{RUN_SELECT} WHERE id = ?1"),
                params![run_id],
                row_to_run,
            )
            .with_context(|| format!("run {run_id} not found"))
    }

    pub fn run_payload(&self, run_id: &str) -> Result<Option<String>> {
        Ok(self.conn.query_row(
            "SELECT payload FROM runs WHERE id = ?1",
            params![run_id],
            |r| r.get(0),
        )?)
    }

    pub fn list_runs(&self, task: Option<&str>, state: Option<&str>) -> Result<Vec<RunRow>> {
        let mut sql = format!("{RUN_SELECT} WHERE 1=1");
        let mut args: Vec<String> = Vec::new();
        if let Some(t) = task {
            sql.push_str(&format!(" AND task = ?{}", args.len() + 1));
            args.push(t.to_string());
        }
        if let Some(s) = state {
            sql.push_str(&format!(" AND state = ?{}", args.len() + 1));
            args.push(s.to_string());
        }
        sql.push_str(" ORDER BY created_at DESC LIMIT 200");
        let mut stmt = self.conn.prepare(&sql)?;
        let rows = stmt
            .query_map(rusqlite::params_from_iter(args), row_to_run)?
            .collect::<rusqlite::Result<Vec<_>>>()?;
        Ok(rows)
    }

    pub fn runs_in_state(&self, state: &str) -> Result<Vec<RunRow>> {
        self.list_runs(None, Some(state))
    }
    pub fn pending_runs_oldest_first(&self) -> Result<Vec<RunRow>> {
        let mut stmt = self.conn.prepare(&format!(
            "{RUN_SELECT} WHERE state = 'pending' ORDER BY created_at ASC"
        ))?;
        let rows = stmt
            .query_map([], row_to_run)?
            .collect::<rusqlite::Result<Vec<_>>>()?;
        Ok(rows)
    }

    pub fn attempts(&self, run_id: &str) -> Result<Vec<AttemptRow>> {
        let mut stmt = self.conn.prepare(
            "SELECT id, run_id, n, agent_name, agent_version, harness, model, phase,
               outcome, error, exit_code, tokens_in, tokens_out, turns, cost_usd,
               artifact_dir, started_at, ended_at
             FROM attempts WHERE run_id = ?1 ORDER BY n",
        )?;
        let rows = stmt
            .query_map(params![run_id], |r| {
                Ok(AttemptRow {
                    id: r.get(0)?,
                    run_id: r.get(1)?,
                    n: r.get(2)?,
                    agent_name: r.get(3)?,
                    agent_version: r.get(4)?,
                    harness: r.get(5)?,
                    model: r.get(6)?,
                    phase: r.get(7)?,
                    outcome: r.get(8)?,
                    error: r.get(9)?,
                    exit_code: r.get(10)?,
                    tokens_in: r.get(11)?,
                    tokens_out: r.get(12)?,
                    turns: r.get(13)?,
                    cost_usd: r.get(14)?,
                    artifact_dir: r.get(15)?,
                    started_at: r.get(16)?,
                    ended_at: r.get(17)?,
                })
            })?
            .collect::<rusqlite::Result<Vec<_>>>()?;
        Ok(rows)
    }

    pub fn events(&self, run_id: &str) -> Result<Vec<RunEventRow>> {
        let mut stmt = self.conn.prepare(
            "SELECT run_id, kind, data, at FROM run_events WHERE run_id = ?1 ORDER BY id",
        )?;
        let rows = stmt
            .query_map(params![run_id], |r| {
                Ok(RunEventRow {
                    run_id: r.get(0)?,
                    kind: r.get(1)?,
                    data: r.get(2)?,
                    at: r.get(3)?,
                })
            })?
            .collect::<rusqlite::Result<Vec<_>>>()?;
        Ok(rows)
    }

    pub fn ingress_event_count(&self, task: &str) -> Result<i64> {
        Ok(self.conn.query_row(
            "SELECT COUNT(*) FROM ingress_events WHERE task = ?1",
            params![task],
            |r| r.get(0),
        )?)
    }
}

const RUN_SELECT: &str = "SELECT id, task, trigger_kind, idempotency_key, state, state_reason,
  trace_id, parent_run_id, agent_name, agent_version, config_source_repo, config_source_ref,
  cost_usd, duration_ms,
  created_at, updated_at FROM runs";

const DLQ_SELECT: &str = "SELECT id, run_id, task, payload, error, created_at,
  replayed_run_id, acknowledged_reason, acknowledged_at FROM dead_letters";

fn row_to_run(r: &rusqlite::Row<'_>) -> rusqlite::Result<RunRow> {
    Ok(RunRow {
        id: r.get(0)?,
        task: r.get(1)?,
        trigger_kind: r.get(2)?,
        idempotency_key: r.get(3)?,
        state: r.get(4)?,
        state_reason: r.get(5)?,
        trace_id: r.get(6)?,
        parent_run_id: r.get(7)?,
        agent_name: r.get(8)?,
        agent_version: r.get(9)?,
        config_source_repo: r.get(10)?,
        config_source_ref: r.get(11)?,
        cost_usd: r.get(12)?,
        duration_ms: r.get(13)?,
        created_at: r.get(14)?,
        updated_at: r.get(15)?,
    })
}

fn row_to_dlq(r: &rusqlite::Row<'_>) -> rusqlite::Result<DeadLetterRow> {
    let replayed: Option<String> = r.get(6)?;
    let acknowledged_at: Option<String> = r.get(8)?;
    Ok(DeadLetterRow {
        id: r.get(0)?,
        run_id: r.get(1)?,
        task: r.get(2)?,
        payload: r.get(3)?,
        error: r.get(4)?,
        created_at: r.get(5)?,
        status: dlq_status(replayed.as_deref(), acknowledged_at.as_deref()),
        replayed_run_id: replayed,
        acknowledged_reason: r.get(7)?,
        acknowledged_at,
    })
}

fn ensure_column(conn: &Connection, table: &str, column: &str, ty: &str) -> Result<()> {
    let mut stmt = conn.prepare(&format!("PRAGMA table_info({table})"))?;
    let cols = stmt
        .query_map([], |r| r.get::<_, String>(1))?
        .collect::<rusqlite::Result<Vec<_>>>()?;
    if !cols.iter().any(|c| c == column) {
        conn.execute(&format!("ALTER TABLE {table} ADD COLUMN {column} {ty}"), [])?;
    }
    Ok(())
}

pub struct IngressRequest<'a> {
    pub task: &'a str,
    pub trigger_kind: &'a str,
    pub idempotency_key: Option<&'a str>,
    pub source_event_id: Option<&'a str>,
    pub payload: Option<&'a str>,
    pub parent_run_id: Option<&'a str>,
}

pub struct IngressOutcome {
    pub run_id: String,
    pub duplicate: bool,
    pub state: String,
}

#[derive(Default)]
pub struct AttemptStats {
    pub tokens_in: Option<i64>,
    pub tokens_out: Option<i64>,
    pub turns: Option<i64>,
    pub cost_usd: Option<f64>,
}

pub fn new_id() -> String {
    uuid::Uuid::new_v4().simple().to_string()[..12].to_string()
}

fn payload_hash(payload: &str) -> String {
    use sha2::{Digest, Sha256};
    let mut h = Sha256::new();
    h.update(payload.as_bytes());
    format!("{:x}", h.finalize())
}
