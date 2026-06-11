//! The durable run ledger (SQLite, WAL). A run row exists before any
//! trigger gets its ack; everything the operator can see flows from here.

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

/// Run states. `blocked_budget` is set at ingress for parked tasks and
/// unparks back to `pending`; `awaiting_recovery` requires an operator.
pub const RUN_STATES: &[&str] = &[
    "pending",
    "running",
    "success",
    "failure",
    "awaiting_recovery",
    "blocked_budget",
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
            | ("awaiting_recovery", "success")
            | ("awaiting_recovery", "failure")
    )
}

/// Attempt phase checkpoints, in order. Failures before `executing` are
/// mechanically retryable; at or after it, recovery is an operator act.
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

#[derive(Debug, Serialize)]
pub struct RunEventRow {
    pub run_id: String,
    pub kind: String,
    pub data: Option<String>,
    pub at: String,
}

#[derive(Debug, Serialize)]
pub struct DeadLetterRow {
    pub id: i64,
    pub run_id: String,
    pub task: String,
    pub payload: Option<String>,
    pub error: String,
    pub created_at: String,
    pub replayed_run_id: Option<String>,
}

pub struct Ledger {
    // Visible to submit.rs, which keeps submission/verdict/gate data
    // mechanics in their own module on the same connection.
    pub(crate) conn: Connection,
}

const SCHEMA: &str = "
CREATE TABLE IF NOT EXISTS runs (
  id TEXT PRIMARY KEY,
  task TEXT NOT NULL,
  trigger_kind TEXT NOT NULL,
  idempotency_key TEXT,
  state TEXT NOT NULL,
  state_reason TEXT,
  trace_id TEXT NOT NULL,
  parent_run_id TEXT,
  agent_name TEXT,
  agent_version INTEGER,
  payload TEXT,
  cost_usd REAL,
  duration_ms INTEGER,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS runs_idempotency
  ON runs(task, idempotency_key) WHERE idempotency_key IS NOT NULL;

CREATE TABLE IF NOT EXISTS ingress_events (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  run_id TEXT,
  task TEXT NOT NULL,
  trigger_kind TEXT NOT NULL,
  source_event_id TEXT,
  dedupe_key TEXT,
  payload_hash TEXT,
  duplicate INTEGER NOT NULL DEFAULT 0,
  received_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS attempts (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  run_id TEXT NOT NULL REFERENCES runs(id),
  n INTEGER NOT NULL,
  agent_name TEXT NOT NULL,
  agent_version INTEGER NOT NULL,
  harness TEXT NOT NULL,
  model TEXT NOT NULL,
  phase TEXT NOT NULL,
  outcome TEXT,
  error TEXT,
  exit_code INTEGER,
  tokens_in INTEGER,
  tokens_out INTEGER,
  turns INTEGER,
  cost_usd REAL,
  artifact_dir TEXT,
  started_at TEXT NOT NULL,
  ended_at TEXT
);

CREATE TABLE IF NOT EXISTS run_events (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  run_id TEXT NOT NULL,
  kind TEXT NOT NULL,
  data TEXT,
  at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS dead_letters (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  run_id TEXT NOT NULL,
  task TEXT NOT NULL,
  payload TEXT,
  error TEXT NOT NULL,
  created_at TEXT NOT NULL,
  replayed_run_id TEXT
);

CREATE TABLE IF NOT EXISTS budget_events (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  task TEXT,
  kind TEXT NOT NULL,
  detail TEXT,
  at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS parked_tasks (
  task TEXT PRIMARY KEY,
  reason TEXT NOT NULL,
  at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS host_leases (
  host TEXT PRIMARY KEY,
  run_id TEXT NOT NULL,
  acquired_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS submissions (
  id TEXT PRIMARY KEY,
  change_key TEXT NOT NULL,
  rev TEXT NOT NULL,
  round INTEGER NOT NULL,
  state TEXT NOT NULL,
  context TEXT,
  prior_report_json TEXT,
  report_json TEXT,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS submissions_one_open
  ON submissions(change_key) WHERE state = 'open';

CREATE TABLE IF NOT EXISTS verdicts (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  submission_id TEXT NOT NULL REFERENCES submissions(id),
  run_id TEXT NOT NULL,
  kind TEXT NOT NULL,
  verdict TEXT NOT NULL,
  findings_json TEXT NOT NULL,
  created_at TEXT NOT NULL,
  UNIQUE (submission_id, kind)
);

CREATE TABLE IF NOT EXISTS rejections (
  change_key TEXT NOT NULL,
  fingerprint TEXT NOT NULL,
  reason TEXT NOT NULL,
  created_at TEXT NOT NULL,
  PRIMARY KEY (change_key, fingerprint)
);
";

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
        Ok(Self { conn })
    }

    #[cfg(test)]
    pub fn open_in_memory() -> Result<Self> {
        let conn = Connection::open_in_memory()?;
        conn.execute_batch(SCHEMA)?;
        Ok(Self { conn })
    }

    // ---- ingress -------------------------------------------------------

    /// Idempotent ingress: every delivery records an `ingress_events` row;
    /// a duplicate dedupe key resolves to the existing run and is never
    /// re-dispatched. The run row is durable before the caller acks.
    pub fn ingest(&mut self, req: IngressRequest<'_>) -> Result<IngressOutcome> {
        // IMMEDIATE: take the write lock up front so concurrent
        // redeliveries serialize instead of racing the dedupe check.
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
        // ON CONFLICT DO NOTHING against the partial unique index makes the
        // dedupe atomic even across processes.
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

    // ---- run state machine ----------------------------------------------

    pub fn run_state(&self, run_id: &str) -> Result<String> {
        self.conn
            .query_row(
                "SELECT state FROM runs WHERE id = ?1",
                params![run_id],
                |r| r.get(0),
            )
            .with_context(|| format!("run {run_id} not found"))
    }

    /// Atomic compare-and-set transition: the UPDATE only fires when the
    /// current state is a legal source for `to`, so two workers can never
    /// both claim the same run. Returns false when the run was not in a
    /// legal source state (already claimed, already terminal).
    pub fn try_transition(&self, run_id: &str, to: &str, reason: Option<&str>) -> Result<bool> {
        let sources: Vec<&str> = RUN_STATES
            .iter()
            .copied()
            .filter(|from| transition_allowed(from, to))
            .collect();
        if sources.is_empty() {
            bail!("no legal transition into state '{to}'");
        }
        // The IN-list is built from our own state constants, not input.
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

    /// Enforced state transition; illegal moves are bugs, not data.
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

    pub fn record_event(&self, run_id: &str, kind: &str, data: Option<&str>) -> Result<()> {
        self.conn.execute(
            "INSERT INTO run_events (run_id, kind, data, at) VALUES (?1, ?2, ?3, ?4)",
            params![run_id, kind, data, now()],
        )?;
        Ok(())
    }

    // ---- attempts --------------------------------------------------------

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

    // ---- host leases ------------------------------------------------------

    /// Durable host lease keyed by substrate resource identity. Returns
    /// false when the host is already leased (caller waits or requeues).
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

    // ---- dead letters ------------------------------------------------------

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
                "SELECT id, run_id, task, payload, error, created_at, replayed_run_id
                 FROM dead_letters WHERE id = ?1",
                params![id],
                |r| {
                    Ok(DeadLetterRow {
                        id: r.get(0)?,
                        run_id: r.get(1)?,
                        task: r.get(2)?,
                        payload: r.get(3)?,
                        error: r.get(4)?,
                        created_at: r.get(5)?,
                        replayed_run_id: r.get(6)?,
                    })
                },
            )
            .with_context(|| format!("dead letter {id} not found"))
    }

    pub fn list_dead_letters(&self) -> Result<Vec<DeadLetterRow>> {
        let mut stmt = self.conn.prepare(
            "SELECT id, run_id, task, payload, error, created_at, replayed_run_id
             FROM dead_letters ORDER BY id DESC",
        )?;
        let rows = stmt
            .query_map([], |r| {
                Ok(DeadLetterRow {
                    id: r.get(0)?,
                    run_id: r.get(1)?,
                    task: r.get(2)?,
                    payload: r.get(3)?,
                    error: r.get(4)?,
                    created_at: r.get(5)?,
                    replayed_run_id: r.get(6)?,
                })
            })?
            .collect::<rusqlite::Result<Vec<_>>>()?;
        Ok(rows)
    }

    /// Atomic claim: only the first marker wins; a concurrent replay of
    /// the same dead letter sees false and must not dispatch.
    pub fn mark_dead_letter_replayed(&self, id: i64, new_run_id: &str) -> Result<bool> {
        let changed = self.conn.execute(
            "UPDATE dead_letters SET replayed_run_id = ?2
             WHERE id = ?1 AND (replayed_run_id IS NULL OR replayed_run_id = ?2)",
            params![id, new_run_id],
        )?;
        Ok(changed == 1)
    }

    /// Free every host lease held by a run (operator resolution path).
    pub fn release_leases_for_run(&self, run_id: &str) -> Result<()> {
        self.conn
            .execute("DELETE FROM host_leases WHERE run_id = ?1", params![run_id])?;
        Ok(())
    }

    // ---- budgets & parking ---------------------------------------------------

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

    /// Runs that actually began dispatch today — pending (not yet
    /// dispatched) and blocked ingress don't consume the daily budget.
    pub fn runs_today(&self, task: &str) -> Result<i64> {
        let day = &now()[..10];
        Ok(self.conn.query_row(
            "SELECT COUNT(*) FROM runs WHERE task = ?1
             AND state NOT IN ('blocked_budget', 'pending')
             AND substr(created_at, 1, 10) = ?2",
            params![task, day],
            |r| r.get(0),
        )?)
    }

    /// Daily spend sums attempts, not runs: a failed run's tokens were
    /// still spent and must count against the ceiling.
    pub fn cost_today(&self) -> Result<f64> {
        let day = &now()[..10];
        Ok(self.conn.query_row(
            "SELECT COALESCE(SUM(cost_usd), 0.0) FROM attempts WHERE substr(started_at, 1, 10) = ?1",
            params![day],
            |r| r.get(0),
        )?)
    }

    // ---- queries -----------------------------------------------------------

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

    /// Dispatch order: oldest pending first, across all tasks.
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
  trace_id, parent_run_id, agent_name, agent_version, cost_usd, duration_ms,
  created_at, updated_at FROM runs";

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
        cost_usd: r.get(10)?,
        duration_ms: r.get(11)?,
        created_at: r.get(12)?,
        updated_at: r.get(13)?,
    })
}

pub struct IngressRequest<'a> {
    pub task: &'a str,
    pub trigger_kind: &'a str,
    pub idempotency_key: Option<&'a str>,
    pub source_event_id: Option<&'a str>,
    pub payload: Option<&'a str>,
    pub parent_run_id: Option<&'a str>,
}

#[derive(Debug)]
pub struct IngressOutcome {
    pub run_id: String,
    pub duplicate: bool,
    pub state: String,
}

#[derive(Debug, Default)]
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

#[cfg(test)]
mod tests {
    use super::*;

    fn ingest_manual(ledger: &mut Ledger, task: &str, key: Option<&str>) -> IngressOutcome {
        ledger
            .ingest(IngressRequest {
                task,
                trigger_kind: "manual",
                idempotency_key: key,
                source_event_id: None,
                payload: Some("{\"x\":1}"),
                parent_run_id: None,
            })
            .unwrap()
    }

    #[test]
    fn duplicate_idempotency_key_creates_one_run_two_ingress_events() {
        let mut ledger = Ledger::open_in_memory().unwrap();
        let first = ingest_manual(&mut ledger, "demo", Some("X"));
        let second = ingest_manual(&mut ledger, "demo", Some("X"));
        assert!(!first.duplicate);
        assert!(second.duplicate);
        assert_eq!(first.run_id, second.run_id);
        assert_eq!(ledger.list_runs(Some("demo"), None).unwrap().len(), 1);
        assert_eq!(ledger.ingress_event_count("demo").unwrap(), 2);
    }

    #[test]
    fn same_key_different_task_is_not_a_duplicate() {
        let mut ledger = Ledger::open_in_memory().unwrap();
        let a = ingest_manual(&mut ledger, "demo", Some("X"));
        let b = ingest_manual(&mut ledger, "other", Some("X"));
        assert!(!b.duplicate);
        assert_ne!(a.run_id, b.run_id);
    }

    #[test]
    fn keyless_ingress_always_creates_a_run() {
        let mut ledger = Ledger::open_in_memory().unwrap();
        let a = ingest_manual(&mut ledger, "demo", None);
        let b = ingest_manual(&mut ledger, "demo", None);
        assert_ne!(a.run_id, b.run_id);
        assert_eq!(ledger.list_runs(Some("demo"), None).unwrap().len(), 2);
    }

    #[test]
    fn legal_lifecycle_transitions() {
        let mut ledger = Ledger::open_in_memory().unwrap();
        let run = ingest_manual(&mut ledger, "demo", None).run_id;
        assert_eq!(ledger.run_state(&run).unwrap(), "pending");
        ledger.transition(&run, "running", None).unwrap();
        ledger.transition(&run, "success", None).unwrap();
        assert_eq!(ledger.run_state(&run).unwrap(), "success");
    }

    #[test]
    fn terminal_states_reject_transitions() {
        let mut ledger = Ledger::open_in_memory().unwrap();
        let run = ingest_manual(&mut ledger, "demo", None).run_id;
        ledger.transition(&run, "running", None).unwrap();
        ledger.transition(&run, "failure", Some("boom")).unwrap();
        for to in ["running", "success", "pending"] {
            assert!(
                ledger.transition(&run, to, None).is_err(),
                "failure -> {to} must be illegal"
            );
        }
    }

    #[test]
    fn pending_cannot_jump_to_success() {
        let mut ledger = Ledger::open_in_memory().unwrap();
        let run = ingest_manual(&mut ledger, "demo", None).run_id;
        assert!(ledger.transition(&run, "success", None).is_err());
    }

    #[test]
    fn awaiting_recovery_resolves_by_operator() {
        let mut ledger = Ledger::open_in_memory().unwrap();
        let run = ingest_manual(&mut ledger, "demo", None).run_id;
        ledger.transition(&run, "running", None).unwrap();
        ledger
            .transition(&run, "awaiting_recovery", Some("orphaned"))
            .unwrap();
        ledger
            .transition(&run, "success", Some("verified by operator"))
            .unwrap();
    }

    #[test]
    fn parked_task_ingress_is_blocked_and_unpark_releases() {
        let mut ledger = Ledger::open_in_memory().unwrap();
        ledger.park_task("demo", "budget breach").unwrap();
        let outcome = ingest_manual(&mut ledger, "demo", None);
        assert_eq!(outcome.state, "blocked_budget");
        let released = ledger.unpark_task("demo").unwrap();
        assert_eq!(released, vec![outcome.run_id.clone()]);
        assert_eq!(ledger.run_state(&outcome.run_id).unwrap(), "pending");
    }

    #[test]
    fn host_lease_is_exclusive_until_released() {
        let ledger = Ledger::open_in_memory().unwrap();
        assert!(ledger.try_acquire_host_lease("sprite-1", "run-a").unwrap());
        assert!(!ledger.try_acquire_host_lease("sprite-1", "run-b").unwrap());
        assert_eq!(
            ledger.lease_holder("sprite-1").unwrap().as_deref(),
            Some("run-a")
        );
        ledger.release_host_lease("sprite-1", "run-a").unwrap();
        assert!(ledger.try_acquire_host_lease("sprite-1", "run-b").unwrap());
    }

    #[test]
    fn dead_letter_replay_lineage() {
        let mut ledger = Ledger::open_in_memory().unwrap();
        let run = ingest_manual(&mut ledger, "demo", None).run_id;
        let dl = ledger
            .record_dead_letter(&run, "demo", Some("{}"), "host unreachable")
            .unwrap();
        let replay = ledger
            .ingest(IngressRequest {
                task: "demo",
                trigger_kind: "replay",
                idempotency_key: Some("replay:1:abc"),
                source_event_id: None,
                payload: Some("{}"),
                parent_run_id: Some(&run),
            })
            .unwrap();
        ledger
            .mark_dead_letter_replayed(dl, &replay.run_id)
            .unwrap();
        let row = ledger.dead_letter(dl).unwrap();
        assert_eq!(row.replayed_run_id.as_deref(), Some(replay.run_id.as_str()));
        let replay_row = ledger.run(&replay.run_id).unwrap();
        assert_eq!(replay_row.parent_run_id.as_deref(), Some(run.as_str()));
    }

    #[test]
    fn claim_is_atomic_second_claimer_loses() {
        let mut ledger = Ledger::open_in_memory().unwrap();
        let run = ingest_manual(&mut ledger, "demo", None).run_id;
        assert!(ledger.try_transition(&run, "running", None).unwrap());
        // A second worker racing for the same pending run must lose.
        assert!(!ledger.try_transition(&run, "running", None).unwrap());
        assert_eq!(ledger.run_state(&run).unwrap(), "running");
    }

    #[test]
    fn dead_letter_replay_claim_is_atomic() {
        let mut ledger = Ledger::open_in_memory().unwrap();
        let run = ingest_manual(&mut ledger, "demo", None).run_id;
        let dl = ledger
            .record_dead_letter(&run, "demo", None, "boom")
            .unwrap();
        assert!(ledger.mark_dead_letter_replayed(dl, "replay-a").unwrap());
        // Same run id is idempotent; a different claimer loses.
        assert!(ledger.mark_dead_letter_replayed(dl, "replay-a").unwrap());
        assert!(!ledger.mark_dead_letter_replayed(dl, "replay-b").unwrap());
    }

    #[test]
    fn cross_handle_ingest_dedupes_on_disk() {
        let dir = tempfile::tempdir().unwrap();
        let db = dir.path().join("plane.db");
        let mut a = Ledger::open(&db).unwrap();
        let mut b = Ledger::open(&db).unwrap();
        let first = ingest_manual(&mut a, "demo", Some("K"));
        let second = ingest_manual(&mut b, "demo", Some("K"));
        assert!(!first.duplicate);
        assert!(second.duplicate);
        assert_eq!(first.run_id, second.run_id);
        assert_eq!(a.ingress_event_count("demo").unwrap(), 2);
    }

    #[test]
    fn attempt_phase_ordering() {
        assert!(phase_reached("executing", "executing"));
        assert!(phase_reached("released", "executing"));
        assert!(!phase_reached("prepared", "executing"));
        assert!(!phase_reached("bogus", "executing"));
    }
}
