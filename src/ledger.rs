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
    "parked_on_ask",
];
pub const EXTERNAL_RUN_STATUSES: &[&str] = &["running", "done", "failed"];

fn transition_allowed(from: &str, to: &str) -> bool {
    matches!(
        (from, to),
        ("pending", "running")
            | ("pending", "failure")
            | ("pending", "blocked_budget")
            | ("running", "success")
            | ("running", "failure")
            | ("running", "awaiting_recovery")
            | ("running", "parked_on_ask")
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
    pub checkout_path: Option<String>,
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

#[derive(Debug)]
pub(crate) struct ArtifactSnapshotRow {
    pub path: String,
    pub size: u64,
    pub content_type: String,
    pub binary: bool,
    pub content: Option<Vec<u8>>,
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

/// bitterblossom-930: one HITL ask raised by a running attempt via `bb ask`.
/// `state`: "open" (raised, not yet answered) -> "answered" (fast path, the
/// still-running attempt's next poll sees it) or -> "parked" (window elapsed
/// with no answer; the owning run has separately finalized as
/// `parked_on_ask`, and answering now creates a resume run instead).
#[derive(Debug, Serialize)]
pub struct AskRow {
    pub id: String,
    pub run_id: String,
    pub task: String,
    pub kind: String,
    pub question: String,
    pub context: Option<String>,
    pub blocking: bool,
    pub window_seconds: i64,
    pub state: String,
    pub answer: Option<String>,
    pub answered_at: Option<String>,
    pub answered_by: Option<String>,
    pub created_at: String,
    pub updated_at: String,
}

#[derive(Debug, Serialize)]
pub struct HostLeaseRow {
    pub host: String,
    pub run_id: String,
    pub acquired_at: String,
}

#[derive(Debug, Serialize)]
pub struct IngressEventRow {
    pub id: i64,
    pub run_id: Option<String>,
    pub task: String,
    pub trigger_kind: String,
    pub source_event_id: Option<String>,
    pub dedupe_key: Option<String>,
    pub payload_hash: Option<String>,
    pub duplicate: bool,
    pub received_at: String,
}

#[derive(Debug, Serialize)]
pub struct GuardEventRow {
    pub id: i64,
    pub kind: String,
    pub task: Option<String>,
    pub detail: Option<String>,
    pub count: i64,
    pub at: String,
}

#[derive(Debug, Serialize)]
pub struct GuardEventCount {
    pub kind: String,
    pub total: i64,
}

#[derive(Debug, Serialize)]
pub struct NotificationOutboxRow {
    pub id: i64,
    pub event: String,
    pub status: String,
    pub attempts: i64,
    pub last_error: Option<String>,
    pub last_status_code: Option<i64>,
    pub last_response: Option<String>,
    pub created_at: String,
    pub updated_at: String,
    pub acknowledged_reason: Option<String>,
    pub acknowledged_at: Option<String>,
}

#[derive(Debug, Serialize)]
pub struct NotificationOutboxCount {
    pub status: String,
    pub total: i64,
}

#[derive(Debug)]
pub struct RetryableNotificationRow {
    pub id: i64,
    pub event: String,
    pub payload: String,
}

#[derive(Debug, Clone, serde::Deserialize)]
pub struct ExternalRunCreate {
    pub agent: String,
    pub role: String,
    pub repo: String,
    pub brief_hash: String,
    pub plane: String,
    pub status_url: Option<String>,
    pub receipt_path: Option<String>,
    pub started_at: String,
}

#[derive(Debug, Serialize)]
pub struct ExternalRunRow {
    pub id: String,
    pub source: String,
    pub agent: String,
    pub role: String,
    pub repo: String,
    pub brief_hash: String,
    pub plane: String,
    pub status: String,
    pub status_url: Option<String>,
    pub receipt_path: Option<String>,
    pub started_at: String,
    pub completed_at: Option<String>,
    pub created_at: String,
    pub updated_at: String,
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
pub const LEDGER_SCHEMA_VERSION: i64 = 1;
type SubmissionHeadRow = (
    String,
    String,
    String,
    i64,
    Option<String>,
    Option<String>,
    Option<String>,
);

impl Ledger {
    pub const MAX_PENDING_QUEUE_DEPTH: i64 = 256;
    pub fn open(path: &Path) -> Result<Self> {
        if let Some(parent) = path.parent() {
            std::fs::create_dir_all(parent)?;
        }
        let conn =
            Connection::open(path).with_context(|| format!("open ledger {}", path.display()))?;
        conn.pragma_update(None, "journal_mode", "WAL")?;
        conn.pragma_update(None, "busy_timeout", 5000_i64)?;
        conn.pragma_update(None, "foreign_keys", "ON")?;
        let existing_version = ledger_schema_version(&conn)?;
        if existing_version > LEDGER_SCHEMA_VERSION {
            bail!(
                "ledger schema version {existing_version} is newer than this bb binary supports ({LEDGER_SCHEMA_VERSION}); roll forward or restore a compatible backup"
            );
        }
        // Keep additive schema changes and their data backfills atomic. If a
        // legacy document cannot migrate, reopening sees the original schema.
        conn.execute_batch("BEGIN IMMEDIATE")?;
        conn.execute_batch(SCHEMA)?;
        ensure_column(&conn, "runs", "config_source_repo", "TEXT")?;
        ensure_column(&conn, "runs", "config_source_ref", "TEXT")?;
        ensure_column(&conn, "dead_letters", "acknowledged_reason", "TEXT")?;
        ensure_column(&conn, "dead_letters", "acknowledged_at", "TEXT")?;
        ensure_column(&conn, "submissions", "head_version", "TEXT")?;
        ensure_column(&conn, "notification_outbox", "last_status_code", "INTEGER")?;
        ensure_column(&conn, "notification_outbox", "last_response", "TEXT")?;
        // bitterblossom-921: the lane checkout/worktree a run's agent created
        // for isolation, if it reported one back. Registered separately from
        // config_source_repo (the stable task-config source) because a lane's
        // own worktree is disposable plane state, not config.
        ensure_column(&conn, "runs", "checkout_path", "TEXT")?;
        // bitterblossom-930: per-run capability token so a dispatched attempt
        // can authenticate its own `bb ask` calls without the operator's
        // global BB_API_TOKEN (least privilege: scoped to this run only).
        ensure_column(&conn, "runs", "ask_token", "TEXT")?;
        // bitterblossom-933: the glass session id for this run's lineage
        // root, stored the first time a post creates one (glass assigns
        // session ids; bb cannot invent its own -- verified live, an
        // unrecognized session_id is a 404, not an auto-create).
        ensure_column(&conn, "runs", "glass_session_id", "TEXT")?;
        // bitterblossom-956: same glass session lineage key for external
        // (register-through) runs -- an interactive session's registration
        // and its done/failed patch cohere into one glass session, exactly
        // like a dispatched run's lifecycle does.
        ensure_column(&conn, "external_runs", "glass_session_id", "TEXT")?;
        // bitterblossom-workflow-runtime-v1: normalized-acceptance dedupe for
        // workflow runs. Column via ensure_column so store-era ledgers
        // migrate additively; the partial unique index must be created after
        // the column exists, so it lives here rather than in schema.sql.
        ensure_column(&conn, "workflow_runs", "dedupe_key", "TEXT")?;
        // Pin the conservative reservation on every accepted workflow run so
        // later policy revisions never revalue queued/running capacity.
        let had_workflow_estimate = column_exists(&conn, "workflow_runs", "estimated_cost_usd")?;
        ensure_column(
            &conn,
            "workflow_runs",
            "estimated_cost_usd",
            "REAL NOT NULL DEFAULT 1.0",
        )?;
        ensure_column(&conn, "workflow_events", "run_id", "TEXT")?;
        // Backfill store-era workflow runs that predate the mutable status table.
        // Runs with step evidence are conservative operator debt; untouched runs
        // remain queued and therefore visible to the bounded worker queue.
        conn.execute(
            "INSERT OR IGNORE INTO workflow_run_status (run_id, state, detail, updated_at)
             SELECT r.id,
                    CASE WHEN EXISTS (
                        SELECT 1 FROM workflow_step_runs s WHERE s.run_id = r.id
                    ) THEN 'needs_attention' ELSE 'queued' END,
                    CASE WHEN EXISTS (
                        SELECT 1 FROM workflow_step_runs s WHERE s.run_id = r.id
                    ) THEN 'legacy run had step evidence but no status row' ELSE NULL END,
                    r.created_at
             FROM workflow_runs r
             WHERE NOT EXISTS (SELECT 1 FROM workflow_run_status s WHERE s.run_id = r.id)",
            [],
        )?;
        conn.execute_batch(
            "CREATE UNIQUE INDEX IF NOT EXISTS workflow_runs_dedupe
               ON workflow_runs(workflow_id, dedupe_key) WHERE dedupe_key IS NOT NULL",
        )?;
        if !had_workflow_estimate {
            backfill_workflow_run_reservations(&conn)?;
        }
        // Store-era ledgers may have accepted rows before the mutable status
        // table existed. Queue those rows so reservations remain visible and
        // the runner can drain them instead of losing them on restart.
        conn.execute(
            "INSERT OR IGNORE INTO workflow_run_status (run_id, state, updated_at)
             SELECT id, 'queued', created_at FROM workflow_runs",
            [],
        )?;
        conn.execute(
            "UPDATE submissions SET head_version = report_json
             WHERE state = 'open' AND head_version IS NULL AND report_json IS NOT NULL",
            [],
        )?;
        conn.pragma_update(None, "user_version", LEDGER_SCHEMA_VERSION)?;
        conn.execute_batch("COMMIT")?;
        Ok(Self { conn })
    }

    pub fn schema_version(&self) -> Result<i64> {
        ledger_schema_version(&self.conn)
    }

    pub fn ingest(&mut self, req: IngressRequest<'_>) -> Result<IngressOutcome> {
        // Dedupe and queue admission share one immediate transaction. The
        // write lock makes the pending-depth check authoritative under a
        // concurrent storm, while the duplicate lookup runs first so a
        // redelivery remains an honest duplicate even when the queue is full.
        let tx = self
            .conn
            .transaction_with_behavior(rusqlite::TransactionBehavior::Immediate)?;
        let ts = now();
        let existing: Option<String> = if let Some(key) = req.idempotency_key {
            tx.query_row(
                "SELECT id FROM runs WHERE task = ?1 AND idempotency_key = ?2",
                params![req.task, key],
                |r| r.get(0),
            )
            .optional()?
        } else {
            None
        };
        if existing.is_none() {
            let depth: i64 = tx.query_row(
                "SELECT COUNT(*) FROM runs WHERE task = ?1 AND state = 'pending'",
                params![req.task],
                |r| r.get(0),
            )?;
            if depth >= Self::MAX_PENDING_QUEUE_DEPTH {
                bail!(
                    "queue backpressure: task '{}' has {depth} pending runs (limit {})",
                    req.task,
                    Self::MAX_PENDING_QUEUE_DEPTH
                );
            }
        }

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

    /// Admit a webhook submission storm as one write transaction.
    ///
    /// Parent dedupe, submission head selection, queue-capacity checks, and
    /// member dedupe/insertion all run under the same IMMEDIATE lock. A
    /// duplicate delivery therefore remains a no-op at a full queue, while a
    /// repair or first admission either inserts the complete tree or nothing.
    pub(crate) fn ingest_submission_storm(
        &mut self,
        parent: IngressRequest<'_>,
        change: &str,
        rev: &str,
        version: Option<&str>,
        members: &[(String, String, String)],
    ) -> Result<IngressOutcome> {
        let tx = self
            .conn
            .transaction_with_behavior(rusqlite::TransactionBehavior::Immediate)?;
        let ts = now();

        let parent_existing: Option<String> = parent
            .idempotency_key
            .and_then(|key| {
                tx.query_row(
                    "SELECT id FROM runs WHERE task = ?1 AND idempotency_key = ?2",
                    params![parent.task, key],
                    |row| row.get(0),
                )
                .optional()
                .transpose()
            })
            .transpose()?;
        let parent_duplicate = parent_existing.is_some();
        let parent_id = parent_existing.unwrap_or_else(new_id);

        let parent_parked: Option<String> = tx
            .query_row(
                "SELECT reason FROM parked_tasks WHERE task = ?1",
                params![parent.task],
                |row| row.get(0),
            )
            .optional()?;
        let mut needed = std::collections::BTreeMap::<String, i64>::new();
        if !parent_duplicate {
            needed.insert(parent.task.to_string(), 1);
        }

        // Resolve the submission head without calling helpers that open their
        // own transaction. This keeps supersession and member admission atomic.
        let latest: Option<SubmissionHeadRow> = tx
            .query_row(
                "SELECT id, state, rev, round, prior_report_json, report_json, head_version
                 FROM submissions WHERE change_key = ?1 ORDER BY rowid DESC LIMIT 1",
                params![change],
                |row| {
                    Ok((
                        row.get(0)?,
                        row.get(1)?,
                        row.get(2)?,
                        row.get(3)?,
                        row.get(4)?,
                        row.get(5)?,
                        row.get(6)?,
                    ))
                },
            )
            .optional()?;
        let mut settle_old = None;
        let mut new_submission: Option<(String, i64, Option<String>)> = None;
        let mut submission_id: Option<String> = None;
        match latest {
            Some((id, state, old_rev, _round, _prior, _report, _old_version))
                if old_rev == rev && state == "open" =>
            {
                submission_id = Some(id);
            }
            Some((_id, _state, old_rev, _round, _prior, _report, _old_version))
                if old_rev == rev => {}
            Some((id, state, _old_rev, _round, _prior, _report, old_version))
                if state == "open" =>
            {
                let newer = version
                    .zip(old_version.as_deref())
                    .is_none_or(|(new, old)| new > old);
                if !parent_duplicate && newer {
                    settle_old = Some(id);
                    let id = new_id();
                    submission_id = Some(id.clone());
                    new_submission = Some((id, 1, None));
                }
            }
            Some((_id, state, _old_rev, round, prior, _report, old_version)) => {
                let newer = version
                    .zip(old_version.as_deref())
                    .is_none_or(|(new, old)| new > old);
                if newer {
                    let id = new_id();
                    submission_id = Some(id.clone());
                    let round = if state == "blocked" { round + 1 } else { 1 };
                    let prior = if state == "blocked" { prior } else { None };
                    new_submission = Some((id, round, prior));
                }
            }
            None => {
                let id = new_id();
                submission_id = Some(id.clone());
                new_submission = Some((id, 1, None));
            }
        }

        let mut member_existing = Vec::with_capacity(members.len());
        if let Some(submission_id) = &submission_id {
            for (task, kind, _payload) in members {
                let key = format!("storm:{submission_id}:{kind}");
                let existing: Option<String> = tx
                    .query_row(
                        "SELECT id FROM runs WHERE task = ?1 AND idempotency_key = ?2",
                        params![task, key],
                        |row| row.get(0),
                    )
                    .optional()?;
                if existing.is_none() {
                    *needed.entry(task.clone()).or_default() += 1;
                }
                member_existing.push(existing);
            }
        }

        // Capacity is charged only for missing rows. Duplicate deliveries
        // can repair nothing and must stay successful even at the ceiling.
        for (task, missing) in &needed {
            let depth: i64 = tx.query_row(
                "SELECT COUNT(*) FROM runs WHERE task = ?1 AND state = 'pending'",
                params![task],
                |row| row.get(0),
            )?;
            if depth + missing > Self::MAX_PENDING_QUEUE_DEPTH {
                bail!(
                    "queue backpressure: task '{}' has {depth} pending runs and needs {missing} additional slots (limit {})",
                    task,
                    Self::MAX_PENDING_QUEUE_DEPTH
                );
            }
        }

        // Parent and event are inserted only after every refusal check above.
        if !parent_duplicate {
            let (state, reason) = match parent_parked {
                Some(reason) => ("blocked_budget", Some(format!("task parked: {reason}"))),
                None => ("pending", None),
            };
            tx.execute(
                "INSERT INTO runs (id, task, trigger_kind, idempotency_key, state,
                   state_reason, trace_id, parent_run_id, payload, created_at, updated_at)
                 VALUES (?1, ?2, ?3, ?4, ?5, ?6, ?7, ?8, ?9, ?10, ?10)",
                params![
                    parent_id,
                    parent.task,
                    parent.trigger_kind,
                    parent.idempotency_key,
                    state,
                    reason,
                    new_id(),
                    parent.parent_run_id,
                    parent.payload,
                    ts
                ],
            )?;
        }
        tx.execute(
            "INSERT INTO ingress_events (run_id, task, trigger_kind, source_event_id,
               dedupe_key, payload_hash, duplicate, received_at)
             VALUES (?1, ?2, ?3, ?4, ?5, ?6, ?7, ?8)",
            params![
                parent_id,
                parent.task,
                parent.trigger_kind,
                parent.source_event_id,
                parent.idempotency_key,
                parent.payload.map(payload_hash),
                parent_duplicate as i64,
                ts
            ],
        )?;

        if let Some(old_id) = settle_old {
            tx.execute(
                "UPDATE submissions SET state = 'abandoned', report_json = '{}', updated_at = ?2
                 WHERE id = ?1 AND state = 'open'",
                params![old_id, ts],
            )?;
        }
        if let Some((id, round, prior_report)) = new_submission {
            tx.execute(
                "INSERT INTO submissions (id, change_key, rev, round, state, context,
                   prior_report_json, created_at, updated_at, head_version)
                 VALUES (?1, ?2, ?3, ?4, 'open', NULL, ?5, ?6, ?6, ?7)",
                params![id, change, rev, round, prior_report, ts, version],
            )?;
        }

        for ((task, kind, payload), existing) in members.iter().zip(member_existing) {
            let Some(submission_id) = submission_id.as_ref() else {
                break;
            };
            let key = format!("storm:{submission_id}:{kind}");
            let payload = serde_json::from_str::<serde_json::Value>(payload)
                .map(|mut value| {
                    value["submission"] = serde_json::Value::String(submission_id.clone());
                    value.to_string()
                })
                .unwrap_or_else(|_| payload.clone());
            let (run_id, duplicate) = if let Some(existing) = existing {
                (existing, true)
            } else {
                let id = new_id();
                let parked: Option<String> = tx
                    .query_row(
                        "SELECT reason FROM parked_tasks WHERE task = ?1",
                        params![task],
                        |row| row.get(0),
                    )
                    .optional()?;
                let (state, reason) = match parked {
                    Some(reason) => ("blocked_budget", Some(format!("task parked: {reason}"))),
                    None => ("pending", None),
                };
                tx.execute(
                    "INSERT INTO runs (id, task, trigger_kind, idempotency_key, state,
                       state_reason, trace_id, parent_run_id, payload, created_at, updated_at)
                     VALUES (?1, ?2, 'webhook', ?3, ?4, ?5, ?6, ?7, ?8, ?9, ?9)",
                    params![
                        id,
                        task,
                        key,
                        state,
                        reason,
                        new_id(),
                        parent_id,
                        payload,
                        ts
                    ],
                )?;
                (id, false)
            };
            tx.execute(
                "INSERT INTO ingress_events (run_id, task, trigger_kind, source_event_id,
                   dedupe_key, payload_hash, duplicate, received_at)
                 VALUES (?1, ?2, 'webhook', NULL, ?3, ?4, ?5, ?6)",
                params![
                    run_id,
                    task,
                    key,
                    payload_hash(&payload),
                    duplicate as i64,
                    ts
                ],
            )?;
        }
        tx.commit()?;

        Ok(IngressOutcome {
            state: self.run_state(&parent_id)?,
            run_id: parent_id,
            duplicate: parent_duplicate,
        })
    }

    /// Remove a newly admitted submission storm tree when member admission
    /// fails. Parent, members, ingress events, and the opened submission are
    /// deleted in one transaction so no partial irreversible admission remains.
    pub fn rollback_ingress_storm(
        &mut self,
        parent_run_id: &str,
        submission_id: Option<&str>,
    ) -> Result<()> {
        let tx = self
            .conn
            .transaction_with_behavior(rusqlite::TransactionBehavior::Immediate)?;
        tx.execute(
            "DELETE FROM ingress_events
             WHERE run_id = ?1 OR run_id IN (SELECT id FROM runs WHERE parent_run_id = ?1)",
            params![parent_run_id],
        )?;
        if let Some(submission_id) = submission_id {
            tx.execute(
                "DELETE FROM submissions WHERE id = ?1",
                params![submission_id],
            )?;
        }
        tx.execute(
            "DELETE FROM runs WHERE id = ?1 OR parent_run_id = ?1",
            params![parent_run_id],
        )?;
        tx.commit()?;
        Ok(())
    }

    pub fn ingest_batch(&mut self, requests: &[IngressRequest<'_>]) -> Result<Vec<IngressOutcome>> {
        let tx = self
            .conn
            .transaction_with_behavior(rusqlite::TransactionBehavior::Immediate)?;
        let ts = now();
        let mut accepted = Vec::with_capacity(requests.len());
        for req in requests {
            let existing: Option<String> = req
                .idempotency_key
                .and_then(|key| {
                    tx.query_row(
                        "SELECT id FROM runs WHERE task = ?1 AND idempotency_key = ?2",
                        params![req.task, key],
                        |r| r.get(0),
                    )
                    .optional()
                    .transpose()
                })
                .transpose()?;
            if existing.is_none() {
                let depth: i64 = tx.query_row(
                    "SELECT COUNT(*) FROM runs WHERE task = ?1 AND state = 'pending'",
                    params![req.task],
                    |r| r.get(0),
                )?;
                if depth >= Self::MAX_PENDING_QUEUE_DEPTH {
                    bail!(
                        "queue backpressure: task '{}' has {depth} pending runs (limit {})",
                        req.task,
                        Self::MAX_PENDING_QUEUE_DEPTH
                    );
                }
            }
            let candidate_id = new_id();
            let trace_id = new_id();
            let parked: Option<String> = tx
                .query_row(
                    "SELECT reason FROM parked_tasks WHERE task = ?1",
                    params![req.task],
                    |r| r.get(0),
                )
                .optional()?;
            let (state, reason) = match parked {
                Some(reason) => ("blocked_budget", Some(format!("task parked: {reason}"))),
                None => ("pending", None),
            };
            let inserted = tx.execute(
                "INSERT INTO runs (id, task, trigger_kind, idempotency_key, state, state_reason, trace_id, parent_run_id, payload, created_at, updated_at)
                 VALUES (?1, ?2, ?3, ?4, ?5, ?6, ?7, ?8, ?9, ?10, ?10)
                 ON CONFLICT(task, idempotency_key) WHERE idempotency_key IS NOT NULL DO NOTHING",
                params![candidate_id, req.task, req.trigger_kind, req.idempotency_key, state, reason, trace_id, req.parent_run_id, req.payload, ts])?;
            let (run_id, duplicate) = if inserted == 1 {
                (candidate_id, false)
            } else {
                let key = req
                    .idempotency_key
                    .context("batch conflict requires idempotency key")?;
                (
                    tx.query_row(
                        "SELECT id FROM runs WHERE task = ?1 AND idempotency_key = ?2",
                        params![req.task, key],
                        |r| r.get(0),
                    )?,
                    true,
                )
            };
            tx.execute(
                "INSERT INTO ingress_events (run_id, task, trigger_kind, source_event_id, dedupe_key, payload_hash, duplicate, received_at)
                 VALUES (?1, ?2, ?3, ?4, ?5, ?6, ?7, ?8)",
                params![run_id, req.task, req.trigger_kind, req.source_event_id, req.idempotency_key, req.payload.map(payload_hash), duplicate as i64, ts])?;
            accepted.push((run_id, duplicate));
        }
        tx.commit()?;
        accepted
            .into_iter()
            .map(|(run_id, duplicate)| {
                Ok(IngressOutcome {
                    state: self.run_state(&run_id)?,
                    run_id,
                    duplicate,
                })
            })
            .collect()
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

    /// Resolve a legacy run without leaving an executing attempt orphaned.
    /// Operator-selected terminal state is recorded only after every open
    /// attempt is closed and its host lease is released.
    pub fn resolve_run(&self, run_id: &str, to: &str, reason: &str) -> Result<()> {
        let from = self.run_state(run_id)?;
        if !transition_allowed(&from, to) {
            bail!("illegal run transition {from} -> {to} for {run_id}");
        }
        let attempts = self.attempts(run_id)?;
        let outcome = if to == "success" {
            "success"
        } else {
            "failure"
        };
        for attempt in attempts.iter().filter(|attempt| attempt.ended_at.is_none()) {
            self.finish_attempt(
                attempt.id,
                outcome,
                Some(reason),
                None,
                &AttemptStats::default(),
                attempt.artifact_dir.as_deref(),
            )?;
            self.set_attempt_phase(attempt.id, "released")?;
        }
        self.transition(run_id, to, Some(reason))?;
        self.release_leases_for_run(run_id)?;
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

    /// Record the lane checkout/worktree path a run's agent created for
    /// isolation, so a later janitor sweep (bitterblossom-921) can find it
    /// once the run reaches a terminal state. Idempotent: last write wins.
    pub fn set_run_checkout_path(&self, run_id: &str, path: &str) -> Result<()> {
        self.conn.execute(
            "UPDATE runs SET checkout_path = ?2, updated_at = ?3 WHERE id = ?1",
            params![run_id, path, now()],
        )?;
        Ok(())
    }

    /// Terminal runs (success/failure/retired) that registered a checkout
    /// path -- candidates for the janitor sweep, before any git-level
    /// safety check (clean tree, fully pushed, age) is applied.
    pub fn runs_with_reapable_checkout(&self) -> Result<Vec<RunRow>> {
        let mut stmt = self.conn.prepare(&format!(
            "{RUN_SELECT} WHERE checkout_path IS NOT NULL
               AND state IN ('success', 'failure', 'retired')"
        ))?;
        let rows = stmt
            .query_map([], row_to_run)?
            .collect::<rusqlite::Result<Vec<_>>>()?;
        Ok(rows)
    }

    pub fn record_event(&self, run_id: &str, kind: &str, data: Option<&str>) -> Result<()> {
        self.conn.execute(
            "INSERT INTO run_events (run_id, kind, data, at) VALUES (?1, ?2, ?3, ?4)",
            params![run_id, kind, data, now()],
        )?;
        Ok(())
    }
    /// Record a lightweight progress marker for `run_id`, separate from run
    /// creation/update time. Used by the dispatcher at attempt phase
    /// transitions and by the foreground heartbeat thread.
    pub fn record_progress(&self, run_id: &str, marker: &str) -> Result<()> {
        self.record_event(run_id, "progress", Some(marker))
    }

    pub fn last_progress_at(&self, run_id: &str) -> Result<Option<String>> {
        Ok(self
            .conn
            .query_row(
                "SELECT at FROM run_events WHERE run_id = ?1 AND kind = 'progress' \
                 ORDER BY id DESC LIMIT 1",
                params![run_id],
                |r| r.get::<_, String>(0),
            )
            .optional()?)
    }

    /// Data of the most recent `boot_probe` event recorded by recovery, or
    /// `None` when the run was never probed.
    pub fn latest_probe(&self, run_id: &str) -> Result<Option<String>> {
        Ok(self
            .conn
            .query_row(
                "SELECT data FROM run_events WHERE run_id = ?1 AND kind = 'boot_probe' \
                 ORDER BY id DESC LIMIT 1",
                params![run_id],
                |r| r.get::<_, Option<String>>(0),
            )
            .optional()?
            .flatten())
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

    pub fn update_attempt_stats(&self, attempt_id: i64, stats: &AttemptStats) -> Result<()> {
        self.conn.execute(
            "UPDATE attempts SET tokens_in = ?2, tokens_out = ?3, turns = ?4, cost_usd = ?5
             WHERE id = ?1",
            params![
                attempt_id,
                stats.tokens_in,
                stats.tokens_out,
                stats.turns,
                stats.cost_usd
            ],
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
        let snapshot_error = artifact_dir
            .and_then(|dir| {
                crate::artifacts::snapshot_attempt(self, attempt_id, Path::new(dir)).err()
            })
            .map(|error| format!("artifact snapshot failed: {error:#}"));
        let requested_success = matches!(outcome, "success" | "parked_on_ask");
        let stored_outcome = if requested_success && snapshot_error.is_some() {
            "failure"
        } else {
            outcome
        };
        let stored_error = match (error, snapshot_error.as_deref()) {
            (Some(error), Some(snapshot_error)) => Some(format!("{error}; {snapshot_error}")),
            (Some(error), None) => Some(error.to_string()),
            (None, Some(snapshot_error)) => Some(snapshot_error.to_string()),
            (None, None) => None,
        };
        self.conn.execute(
            "UPDATE attempts SET outcome = ?2, error = ?3, exit_code = ?4, tokens_in = ?5,
               tokens_out = ?6, turns = ?7, cost_usd = ?8, artifact_dir = ?9, ended_at = ?10
             WHERE id = ?1",
            params![
                attempt_id,
                stored_outcome,
                stored_error,
                exit_code,
                stats.tokens_in,
                stats.tokens_out,
                stats.turns,
                stats.cost_usd,
                artifact_dir,
                now()
            ],
        )?;
        if requested_success {
            if let Some(error) = snapshot_error {
                bail!(error);
            }
        }
        Ok(())
    }

    pub(crate) fn replace_artifact_snapshots(
        &self,
        attempt_id: i64,
        snapshots: &[ArtifactSnapshotRow],
    ) -> Result<()> {
        let tx = self.conn.unchecked_transaction()?;
        tx.execute(
            "DELETE FROM artifact_snapshots WHERE attempt_id = ?1",
            params![attempt_id],
        )?;
        {
            let mut insert = tx.prepare(
                "INSERT INTO artifact_snapshots
                   (attempt_id, path, size, content_type, binary, content)
                 VALUES (?1, ?2, ?3, ?4, ?5, ?6)",
            )?;
            for snapshot in snapshots {
                let size = i64::try_from(snapshot.size)
                    .context("artifact snapshot size exceeds SQLite INTEGER range")?;
                insert.execute(params![
                    attempt_id,
                    snapshot.path,
                    size,
                    snapshot.content_type,
                    snapshot.binary,
                    snapshot.content,
                ])?;
            }
        }
        tx.commit()?;
        Ok(())
    }

    pub(crate) fn artifact_snapshots(&self, attempt_id: i64) -> Result<Vec<ArtifactSnapshotRow>> {
        let mut stmt = self.conn.prepare(
            "SELECT path, size, content_type, binary, content
             FROM artifact_snapshots WHERE attempt_id = ?1 ORDER BY path",
        )?;
        let rows = stmt
            .query_map(params![attempt_id], |row| {
                let size: i64 = row.get(1)?;
                Ok(ArtifactSnapshotRow {
                    path: row.get(0)?,
                    size: u64::try_from(size)
                        .map_err(|_| rusqlite::Error::IntegralValueOutOfRange(1, size))?,
                    content_type: row.get(2)?,
                    binary: row.get(3)?,
                    content: row.get(4)?,
                })
            })?
            .collect::<rusqlite::Result<Vec<_>>>()?;
        Ok(rows)
    }

    pub(crate) fn artifact_snapshot(
        &self,
        attempt_id: i64,
        path: &str,
    ) -> Result<Option<ArtifactSnapshotRow>> {
        self.conn
            .query_row(
                "SELECT path, size, content_type, binary, content
                 FROM artifact_snapshots WHERE attempt_id = ?1 AND path = ?2",
                params![attempt_id, path],
                |row| {
                    let size: i64 = row.get(1)?;
                    Ok(ArtifactSnapshotRow {
                        path: row.get(0)?,
                        size: u64::try_from(size)
                            .map_err(|_| rusqlite::Error::IntegralValueOutOfRange(1, size))?,
                        content_type: row.get(2)?,
                        binary: row.get(3)?,
                        content: row.get(4)?,
                    })
                },
            )
            .optional()
            .map_err(Into::into)
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

    /// Release every lease owned by a run, even when its task/workflow
    /// declaration was renamed or removed after the process crashed.
    pub fn release_host_leases_for_run(&self, run_id: &str) -> Result<usize> {
        Ok(self
            .conn
            .execute("DELETE FROM host_leases WHERE run_id = ?1", params![run_id])?)
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

    pub fn list_host_leases(&self) -> Result<Vec<HostLeaseRow>> {
        let mut stmt = self
            .conn
            .prepare("SELECT host, run_id, acquired_at FROM host_leases ORDER BY acquired_at")?;
        let rows = stmt
            .query_map([], |r| {
                Ok(HostLeaseRow {
                    host: r.get(0)?,
                    run_id: r.get(1)?,
                    acquired_at: r.get(2)?,
                })
            })?
            .collect::<rusqlite::Result<Vec<_>>>()?;
        Ok(rows)
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

    /// Return the complete newest-first DLQ view for operators. Callers that
    /// need bounded work (for example, API widgets) use the explicit page
    /// method; the primary CLI/API list must not silently hide older rows.
    pub fn list_dead_letters(&self) -> Result<Vec<DeadLetterRow>> {
        let mut stmt = self
            .conn
            .prepare(&format!("{DLQ_SELECT} ORDER BY id DESC"))?;
        let rows = stmt
            .query_map([], row_to_dlq)?
            .collect::<rusqlite::Result<Vec<_>>>()?;
        Ok(rows)
    }

    pub fn list_dead_letters_page(&self, limit: i64) -> Result<Vec<DeadLetterRow>> {
        let limit = limit.clamp(1, 200);
        let mut stmt = self
            .conn
            .prepare(&format!("{DLQ_SELECT} ORDER BY id DESC LIMIT ?1"))?;
        let rows = stmt
            .query_map(params![limit], row_to_dlq)?
            .collect::<rusqlite::Result<Vec<_>>>()?;
        Ok(rows)
    }

    pub fn open_dead_letter_count(&self, task: Option<&str>) -> Result<i64> {
        let mut sql = String::from(
            "SELECT COUNT(*) FROM dead_letters
             WHERE acknowledged_at IS NULL AND replayed_run_id IS NULL",
        );
        let mut args = Vec::new();
        if let Some(task) = task {
            sql.push_str(" AND task = ?1");
            args.push(task.to_string());
        }
        let count = self
            .conn
            .query_row(&sql, rusqlite::params_from_iter(args), |row| row.get(0))?;
        Ok(count)
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
        self.conn.execute_batch("BEGIN IMMEDIATE")?;
        let result = (|| {
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
        })();
        match result {
            Ok(row) => {
                self.conn.execute_batch("COMMIT")?;
                Ok(row)
            }
            Err(err) => {
                let _ = self.conn.execute_batch("ROLLBACK");
                Err(err)
            }
        }
    }

    pub fn set_run_ask_token(&self, run_id: &str, token: &str) -> Result<()> {
        self.conn.execute(
            "UPDATE runs SET ask_token = ?2, updated_at = ?3 WHERE id = ?1",
            params![run_id, token, now()],
        )?;
        Ok(())
    }

    pub fn run_ask_token(&self, run_id: &str) -> Result<Option<String>> {
        Ok(self
            .conn
            .query_row(
                "SELECT ask_token FROM runs WHERE id = ?1",
                params![run_id],
                |r| r.get(0),
            )
            .optional()?
            .flatten())
    }

    pub fn set_run_glass_session(&self, run_id: &str, session_id: &str) -> Result<()> {
        self.conn.execute(
            "UPDATE runs SET glass_session_id = ?2, updated_at = ?3 WHERE id = ?1",
            params![run_id, session_id, now()],
        )?;
        Ok(())
    }

    pub fn run_glass_session(&self, run_id: &str) -> Result<Option<String>> {
        Ok(self
            .conn
            .query_row(
                "SELECT glass_session_id FROM runs WHERE id = ?1",
                params![run_id],
                |r| r.get(0),
            )
            .optional()?
            .flatten())
    }

    pub fn set_external_run_glass_session(&self, id: &str, session_id: &str) -> Result<()> {
        self.conn.execute(
            "UPDATE external_runs SET glass_session_id = ?2, updated_at = ?3 WHERE id = ?1",
            params![id, session_id, now()],
        )?;
        Ok(())
    }

    pub fn external_run_glass_session(&self, id: &str) -> Result<Option<String>> {
        Ok(self
            .conn
            .query_row(
                "SELECT glass_session_id FROM external_runs WHERE id = ?1",
                params![id],
                |r| r.get(0),
            )
            .optional()?
            .flatten())
    }

    /// Raise a new ask for a running attempt. `kind` is "question", "decision",
    /// or "approval"; validated by the caller (HTTP route), not here, so the
    /// ledger stays a plain data store.
    #[allow(clippy::too_many_arguments)]
    pub fn raise_ask(
        &self,
        id: &str,
        run_id: &str,
        task: &str,
        kind: &str,
        question: &str,
        context: Option<&str>,
        blocking: bool,
        window_seconds: i64,
    ) -> Result<AskRow> {
        self.conn.execute(
            "INSERT INTO asks (id, run_id, task, kind, question, context, blocking,
               window_seconds, state, created_at, updated_at)
             VALUES (?1, ?2, ?3, ?4, ?5, ?6, ?7, ?8, 'open', ?9, ?9)",
            params![
                id,
                run_id,
                task,
                kind,
                question,
                context,
                blocking as i64,
                window_seconds,
                now()
            ],
        )?;
        self.ask(id)
    }

    pub fn ask(&self, id: &str) -> Result<AskRow> {
        self.conn
            .query_row(
                &format!("{ASK_SELECT} WHERE id = ?1"),
                params![id],
                row_to_ask,
            )
            .with_context(|| format!("ask {id} not found"))
    }

    /// Lazily transition an ask past its window into `parked` if it is still
    /// `open` and unanswered. Idempotent; called on every poll so the CLI's
    /// own poll loop is the only clock this primitive depends on.
    pub fn park_ask_if_expired(&self, id: &str) -> Result<AskRow> {
        self.conn.execute(
            "UPDATE asks SET state = 'parked', updated_at = ?2
             WHERE id = ?1 AND state = 'open'
               AND (unixepoch(?2) - unixepoch(created_at)) >= window_seconds",
            params![id, now()],
        )?;
        self.ask(id)
    }

    pub fn asks_for_run(&self, run_id: &str) -> Result<Vec<AskRow>> {
        let mut stmt = self.conn.prepare(&format!(
            "{ASK_SELECT} WHERE run_id = ?1 ORDER BY created_at ASC"
        ))?;
        let rows = stmt
            .query_map(params![run_id], row_to_ask)?
            .collect::<rusqlite::Result<Vec<_>>>()?;
        Ok(rows)
    }

    /// Record an answer. Works whether the ask is still `open` (the raising
    /// attempt's next poll sees it -- fast path) or already `parked` (the
    /// caller is responsible for creating the resume run; this only records
    /// the answer). Refuses an ask that already has an answer.
    pub fn answer_ask(&self, id: &str, answer: &str, answered_by: &str) -> Result<AskRow> {
        let changed = self.conn.execute(
            "UPDATE asks SET answer = ?2, answered_at = ?3, answered_by = ?4,
               state = 'answered', updated_at = ?3
             WHERE id = ?1 AND answer IS NULL",
            params![id, answer, now(), answered_by],
        )?;
        if changed != 1 {
            let current = self.ask(id)?;
            bail!("ask {id} already answered at {:?}", current.answered_at);
        }
        self.ask(id)
    }

    pub fn blocked_budget_runs_for_task(&self, task: &str) -> Result<Vec<RunRow>> {
        let mut stmt = self.conn.prepare(&format!(
            "{RUN_SELECT} WHERE task = ?1 AND state = 'blocked_budget' \
             ORDER BY created_at ASC, id ASC"
        ))?;
        let rows = stmt
            .query_map(params![task], row_to_run)?
            .collect::<rusqlite::Result<Vec<_>>>()?;
        Ok(rows)
    }

    pub fn unpark_task(&self, task: &str) -> Result<Vec<String>> {
        let blocked: Vec<String> = self
            .blocked_budget_runs_for_task(task)?
            .into_iter()
            .map(|run| run.id)
            .collect();
        self.unpark_task_runs(task, &blocked, "unparked")
    }

    pub fn unpark_task_runs(
        &self,
        task: &str,
        run_ids: &[String],
        reason: &str,
    ) -> Result<Vec<String>> {
        for run_id in run_ids {
            let run = self.require_blocked_budget(run_id)?;
            if run.task != task {
                bail!("run {run_id} belongs to task '{}', not '{task}'", run.task);
            }
        }
        self.conn
            .execute("DELETE FROM parked_tasks WHERE task = ?1", params![task])?;
        for run_id in run_ids {
            self.transition(run_id, "pending", Some(reason))?;
        }
        Ok(run_ids.to_vec())
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
    pub fn standard_cost_today(&self) -> Result<(f64, i64)> {
        let day = &now()[..10];
        Ok(self.conn.query_row(
            "SELECT COALESCE(SUM(cost_usd), 0.0),
                    SUM(CASE WHEN cost_usd IS NULL THEN 1 ELSE 0 END)
             FROM attempts WHERE substr(started_at, 1, 10) = ?1",
            params![day],
            |r| Ok((r.get(0)?, r.get::<_, Option<i64>>(1)?.unwrap_or(0))),
        )?)
    }

    pub fn cost_today(&self) -> Result<f64> {
        Ok(self.standard_cost_today()?.0)
    }

    /// Cost governor slice 1 (bitterblossom-960): sum of today's attempt
    /// cost across every task namespaced under `<repo_prefix>/`, the same
    /// namespace `load_workload_repo_tasks` uses for repo-owned task names.
    /// Backs the repo-scoped daily ceiling, which contains an overspending
    /// repo's blast radius to that repo alone.
    pub fn cost_today_for_repo(&self, repo_prefix: &str) -> Result<f64> {
        let day = &now()[..10];
        let like = format!("{repo_prefix}/%");
        Ok(self.conn.query_row(
            "SELECT COALESCE(SUM(a.cost_usd), 0.0)
             FROM attempts a JOIN runs r ON a.run_id = r.id
             WHERE r.task LIKE ?1 AND substr(a.started_at, 1, 10) = ?2",
            params![like, day],
            |r| r.get(0),
        )?)
    }

    /// Cost governor slice 1 (bitterblossom-960): how many `budget_events`
    /// rows exist today for this exact (task, kind) pair, including the one
    /// `admit_dispatch` just recorded for the current admission. A count of
    /// 1 means this is the first breach of this kind today (escalate); a
    /// count above 1 means every subsequent same-day, same-kind trigger is a
    /// grind repeat (stay silent -- the run row itself still records it).
    pub fn budget_events_today_count(&self, task: &str, kind: &str) -> Result<i64> {
        let day = &now()[..10];
        Ok(self.conn.query_row(
            "SELECT COUNT(*) FROM budget_events
             WHERE task = ?1 AND kind = ?2 AND substr(at, 1, 10) = ?3",
            params![task, kind, day],
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

    pub fn create_external_run(&self, input: ExternalRunCreate) -> Result<ExternalRunRow> {
        validate_external_create(&input)?;
        let id = new_id();
        let ts = now();
        self.conn.execute(
            "INSERT INTO external_runs (id, agent, role, repo, brief_hash, plane, status,
               status_url, receipt_path, started_at, created_at, updated_at)
             VALUES (?1, ?2, ?3, ?4, ?5, ?6, 'running', ?7, ?8, ?9, ?10, ?10)",
            params![
                id,
                input.agent.trim(),
                input.role.trim(),
                input.repo.trim(),
                input.brief_hash.trim(),
                input.plane.trim(),
                option_trimmed(input.status_url.as_deref()),
                option_trimmed(input.receipt_path.as_deref()),
                input.started_at.trim(),
                ts
            ],
        )?;
        self.external_run(&id)
    }

    pub fn update_external_run(
        &self,
        id: &str,
        status: &str,
        completed_at: Option<&str>,
    ) -> Result<ExternalRunRow> {
        validate_external_status(status)?;
        let current = self.external_run(id)?;
        if !external_transition_allowed(&current.status, status) {
            bail!(
                "illegal external run transition {} -> {} for {id}",
                current.status,
                status
            );
        }
        let completed_at = match status {
            "done" | "failed" => {
                let at = completed_at
                    .map(str::trim)
                    .filter(|s| !s.is_empty())
                    .ok_or_else(|| {
                        anyhow::anyhow!("completed_at is required for external status {status}")
                    })?;
                validate_rfc3339("completed_at", at)?;
                Some(at.to_string())
            }
            "running" => None,
            _ => unreachable!("validated external status"),
        };
        self.conn.execute(
            "UPDATE external_runs SET status = ?2, completed_at = ?3, updated_at = ?4
             WHERE id = ?1",
            params![id, status, completed_at, now()],
        )?;
        self.external_run(id)
    }

    pub fn external_run(&self, id: &str) -> Result<ExternalRunRow> {
        self.conn
            .query_row(
                &format!("{EXTERNAL_RUN_SELECT} WHERE id = ?1"),
                params![id],
                row_to_external_run,
            )
            .with_context(|| format!("external run {id} not found"))
    }

    pub fn list_external_runs(&self, limit: i64) -> Result<Vec<ExternalRunRow>> {
        let limit = limit.clamp(1, 200);
        let mut stmt = self.conn.prepare(&format!(
            "{EXTERNAL_RUN_SELECT} ORDER BY created_at DESC LIMIT ?1"
        ))?;
        let rows = stmt
            .query_map(params![limit], row_to_external_run)?
            .collect::<rusqlite::Result<Vec<_>>>()?;
        Ok(rows)
    }

    pub fn runs_in_state(&self, state: &str) -> Result<Vec<RunRow>> {
        self.list_runs(None, Some(state))
    }
    pub const DISPATCH_QUEUE_BATCH: i64 = 64;

    pub fn pending_runs_oldest_first(&self) -> Result<Vec<RunRow>> {
        let mut stmt = self.conn.prepare(&format!(
            "{RUN_SELECT} WHERE state = 'pending' ORDER BY created_at ASC, id ASC LIMIT ?1"
        ))?;
        let rows = stmt
            .query_map(params![Self::DISPATCH_QUEUE_BATCH], row_to_run)?
            .collect::<rusqlite::Result<Vec<_>>>()?;
        Ok(rows)
    }

    pub fn pending_run_depth(&self, task: Option<&str>) -> Result<i64> {
        let mut sql = String::from("SELECT COUNT(*) FROM runs WHERE state = 'pending'");
        let mut args = Vec::new();
        if let Some(task) = task {
            sql.push_str(" AND task = ?1");
            args.push(task.to_string());
        }
        Ok(self
            .conn
            .query_row(&sql, rusqlite::params_from_iter(args), |row| row.get(0))?)
    }

    pub fn oldest_pending_run_at(&self, task: Option<&str>) -> Result<Option<String>> {
        let mut sql = String::from("SELECT created_at FROM runs WHERE state = 'pending'");
        let mut args = Vec::new();
        if let Some(task) = task {
            sql.push_str(" AND task = ?1");
            args.push(task.to_string());
        }
        sql.push_str(" ORDER BY created_at, id LIMIT 1");
        Ok(self
            .conn
            .query_row(&sql, rusqlite::params_from_iter(args), |row| row.get(0))
            .optional()?)
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

    pub fn latest_ingress_events(&self, limit: i64) -> Result<Vec<IngressEventRow>> {
        let limit = limit.clamp(1, 200);
        let mut stmt = self.conn.prepare(
            "SELECT id, run_id, task, trigger_kind, source_event_id, dedupe_key,
               payload_hash, duplicate, received_at
             FROM ingress_events ORDER BY id DESC LIMIT ?1",
        )?;
        let rows = stmt
            .query_map(params![limit], |r| {
                Ok(IngressEventRow {
                    id: r.get(0)?,
                    run_id: r.get(1)?,
                    task: r.get(2)?,
                    trigger_kind: r.get(3)?,
                    source_event_id: r.get(4)?,
                    dedupe_key: r.get(5)?,
                    payload_hash: r.get(6)?,
                    duplicate: r.get::<_, i64>(7)? != 0,
                    received_at: r.get(8)?,
                })
            })?
            .collect::<rusqlite::Result<Vec<_>>>()?;
        Ok(rows)
    }

    /// Record a guardrail event (backlog 083): ingress body rejection, cron
    /// catch-up collapse, notification failure, or plane pause/resume. `count`
    /// lets a collapse record how many fires it skipped in a single row.
    pub fn record_guard_event(
        &self,
        kind: &str,
        task: Option<&str>,
        detail: &str,
        count: i64,
    ) -> Result<()> {
        self.conn.execute(
            "INSERT INTO guard_events (kind, task, detail, count, at) VALUES (?1, ?2, ?3, ?4, ?5)",
            params![kind, task, detail, count, now()],
        )?;
        Ok(())
    }

    pub fn list_guard_events(&self, limit: i64) -> Result<Vec<GuardEventRow>> {
        let limit = limit.clamp(1, 200);
        let mut stmt = self.conn.prepare(
            "SELECT id, kind, task, detail, count, at FROM guard_events
             ORDER BY id DESC LIMIT ?1",
        )?;
        let rows = stmt
            .query_map(params![limit], |r| {
                Ok(GuardEventRow {
                    id: r.get(0)?,
                    kind: r.get(1)?,
                    task: r.get(2)?,
                    detail: r.get(3)?,
                    count: r.get(4)?,
                    at: r.get(5)?,
                })
            })?
            .collect::<rusqlite::Result<Vec<_>>>()?;
        Ok(rows)
    }

    pub fn guard_event_counts(&self) -> Result<Vec<GuardEventCount>> {
        let mut stmt = self
            .conn
            .prepare("SELECT kind, COALESCE(SUM(count), 0) FROM guard_events GROUP BY kind")?;
        let rows = stmt
            .query_map([], |r| {
                Ok(GuardEventCount {
                    kind: r.get(0)?,
                    total: r.get(1)?,
                })
            })?
            .collect::<rusqlite::Result<Vec<_>>>()?;
        Ok(rows)
    }

    pub fn enqueue_notification(&self, event: &str, payload: &str) -> Result<i64> {
        let ts = now();
        self.conn.execute(
            "INSERT INTO notification_outbox (event, payload, status, created_at, updated_at)
             VALUES (?1, ?2, 'pending', ?3, ?3)",
            params![event, payload, ts],
        )?;
        Ok(self.conn.last_insert_rowid())
    }

    pub fn mark_notification_delivered(
        &self,
        id: i64,
        status_code: Option<i64>,
        response: Option<&str>,
    ) -> Result<()> {
        let ts = now();
        self.conn.execute(
            "UPDATE notification_outbox
             SET status = 'delivered', attempts = attempts + 1, last_error = NULL,
                 last_status_code = ?2, last_response = ?3,
                 updated_at = ?4, delivered_at = ?4
             WHERE id = ?1",
            params![id, status_code, response, ts],
        )?;
        Ok(())
    }

    pub fn mark_notification_failed(
        &self,
        id: i64,
        error: &str,
        status_code: Option<i64>,
        response: Option<&str>,
    ) -> Result<()> {
        self.conn.execute(
            "UPDATE notification_outbox
             SET status = 'failed', attempts = attempts + 1, last_error = ?2,
                 last_status_code = ?3, last_response = ?4,
                 updated_at = ?5
             WHERE id = ?1",
            params![id, error, status_code, response, now()],
        )?;
        Ok(())
    }

    pub fn acknowledge_notification(&self, id: i64, reason: &str) -> Result<NotificationOutboxRow> {
        let reason = reason.trim();
        if reason.is_empty() {
            bail!("acknowledging notification {id} requires a non-empty reason");
        }
        let row = self.notification_outbox(id)?;
        match row.status.as_str() {
            "pending" | "failed" => {}
            "acknowledged" => bail!(
                "notification {id} already acknowledged: {}",
                row.acknowledged_reason.as_deref().unwrap_or("-")
            ),
            "delivered" => bail!(
                "notification {id} already delivered; acknowledge is for pending or failed rows"
            ),
            other => bail!("notification {id} is {other}; acknowledge requires pending or failed"),
        }
        let ts = now();
        let changed = self.conn.execute(
            "UPDATE notification_outbox
             SET status = 'acknowledged', acknowledged_reason = ?2,
                 acknowledged_at = ?3, updated_at = ?3
             WHERE id = ?1 AND status IN ('pending', 'failed')",
            params![id, reason, ts],
        )?;
        if changed != 1 {
            bail!("notification {id} changed before acknowledgement");
        }
        self.notification_outbox(id)
    }

    pub fn notification_outbox(&self, id: i64) -> Result<NotificationOutboxRow> {
        self.conn
            .query_row(
                "SELECT id, event, status, attempts, last_error, last_status_code,
                        last_response, created_at, updated_at,
                        acknowledged_reason, acknowledged_at
                 FROM notification_outbox WHERE id = ?1",
                params![id],
                row_to_notification_outbox,
            )
            .with_context(|| format!("notification {id} not found"))
    }

    pub fn list_notification_outbox(&self, limit: i64) -> Result<Vec<NotificationOutboxRow>> {
        let limit = limit.clamp(1, 200);
        let mut stmt = self.conn.prepare(
            "SELECT id, event, status, attempts, last_error, last_status_code,
                    last_response, created_at, updated_at,
                    acknowledged_reason, acknowledged_at
             FROM notification_outbox ORDER BY id DESC LIMIT ?1",
        )?;
        let rows = stmt
            .query_map(params![limit], row_to_notification_outbox)?
            .collect::<rusqlite::Result<Vec<_>>>()?;
        Ok(rows)
    }

    pub fn retryable_notifications(&self, limit: i64) -> Result<Vec<RetryableNotificationRow>> {
        let limit = limit.clamp(1, 200);
        let mut stmt = self.conn.prepare(
            "SELECT id, event, payload FROM notification_outbox
             WHERE status IN ('pending', 'failed')
             ORDER BY id ASC LIMIT ?1",
        )?;
        let rows = stmt
            .query_map(params![limit], |r| {
                Ok(RetryableNotificationRow {
                    id: r.get(0)?,
                    event: r.get(1)?,
                    payload: r.get(2)?,
                })
            })?
            .collect::<rusqlite::Result<Vec<_>>>()?;
        Ok(rows)
    }

    pub fn notification_outbox_counts(&self) -> Result<Vec<NotificationOutboxCount>> {
        let mut stmt = self.conn.prepare(
            "SELECT status, COUNT(*) FROM notification_outbox GROUP BY status ORDER BY status",
        )?;
        let rows = stmt
            .query_map([], |r| {
                Ok(NotificationOutboxCount {
                    status: r.get(0)?,
                    total: r.get(1)?,
                })
            })?
            .collect::<rusqlite::Result<Vec<_>>>()?;
        Ok(rows)
    }

    /// Pause reflex dispatch for the whole plane (backlog 083). Distinct from
    /// per-task parking: a pause stops the autonomous dispatch loop, not one
    /// task's budget. Manual `bb run` still dispatches — pause is a reflex
    /// circuit breaker, not an operator lock.
    pub fn pause_plane(&self, reason: &str) -> Result<()> {
        self.conn.execute(
            "INSERT INTO plane_pause (row, reason, at) VALUES (1, ?2, ?3)
             ON CONFLICT(row) DO UPDATE SET reason = ?2, at = ?3",
            params![1, reason, now()],
        )?;
        Ok(())
    }

    pub fn resume_plane(&self) -> Result<bool> {
        let changed = self
            .conn
            .execute("DELETE FROM plane_pause WHERE row = 1", [])?;
        Ok(changed == 1)
    }

    pub fn plane_paused(&self) -> Result<Option<(String, String)>> {
        Ok(self
            .conn
            .query_row(
                "SELECT reason, at FROM plane_pause WHERE row = 1",
                [],
                |r| Ok((r.get::<_, String>(0)?, r.get::<_, String>(1)?)),
            )
            .optional()?)
    }

    /// Spent so far on attempts belonging to currently-running runs. Streaming
    /// harnesses update the running attempt while it executes; final-only
    /// harnesses contribute once they finish. Pairs with reserved spend in
    /// status.
    pub fn in_flight_cost(&self) -> Result<f64> {
        Ok(self.conn.query_row(
            "SELECT COALESCE(SUM(a.cost_usd), 0.0) FROM attempts a
             JOIN runs r ON r.id = a.run_id WHERE r.state = 'running'",
            [],
            |r| r.get(0),
        )?)
    }
}

const RUN_SELECT: &str = "SELECT id, task, trigger_kind, idempotency_key, state, state_reason,
  trace_id, parent_run_id, agent_name, agent_version, config_source_repo, config_source_ref,
  checkout_path, cost_usd, duration_ms,
  created_at, updated_at FROM runs";

const EXTERNAL_RUN_SELECT: &str = "SELECT id, agent, role, repo, brief_hash, plane, status,
  status_url, receipt_path, started_at, completed_at, created_at, updated_at FROM external_runs";

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
        checkout_path: r.get(12)?,
        cost_usd: r.get(13)?,
        duration_ms: r.get(14)?,
        created_at: r.get(15)?,
        updated_at: r.get(16)?,
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

const ASK_SELECT: &str = "SELECT id, run_id, task, kind, question, context, blocking,
  window_seconds, state, answer, answered_at, answered_by, created_at, updated_at FROM asks";

fn row_to_ask(r: &rusqlite::Row<'_>) -> rusqlite::Result<AskRow> {
    Ok(AskRow {
        id: r.get(0)?,
        run_id: r.get(1)?,
        task: r.get(2)?,
        kind: r.get(3)?,
        question: r.get(4)?,
        context: r.get(5)?,
        blocking: r.get::<_, i64>(6)? != 0,
        window_seconds: r.get(7)?,
        state: r.get(8)?,
        answer: r.get(9)?,
        answered_at: r.get(10)?,
        answered_by: r.get(11)?,
        created_at: r.get(12)?,
        updated_at: r.get(13)?,
    })
}

fn row_to_external_run(r: &rusqlite::Row<'_>) -> rusqlite::Result<ExternalRunRow> {
    Ok(ExternalRunRow {
        id: r.get(0)?,
        source: "external".to_string(),
        agent: r.get(1)?,
        role: r.get(2)?,
        repo: r.get(3)?,
        brief_hash: r.get(4)?,
        plane: r.get(5)?,
        status: r.get(6)?,
        status_url: r.get(7)?,
        receipt_path: r.get(8)?,
        started_at: r.get(9)?,
        completed_at: r.get(10)?,
        created_at: r.get(11)?,
        updated_at: r.get(12)?,
    })
}

fn row_to_notification_outbox(r: &rusqlite::Row<'_>) -> rusqlite::Result<NotificationOutboxRow> {
    Ok(NotificationOutboxRow {
        id: r.get(0)?,
        event: r.get(1)?,
        status: r.get(2)?,
        attempts: r.get(3)?,
        last_error: r.get(4)?,
        last_status_code: r.get(5)?,
        last_response: r.get(6)?,
        created_at: r.get(7)?,
        updated_at: r.get(8)?,
        acknowledged_reason: r.get(9)?,
        acknowledged_at: r.get(10)?,
    })
}

fn column_exists(conn: &Connection, table: &str, column: &str) -> Result<bool> {
    let mut stmt = conn.prepare(&format!("PRAGMA table_info({table})"))?;
    let cols = stmt
        .query_map([], |r| r.get::<_, String>(1))?
        .collect::<rusqlite::Result<Vec<_>>>()?;
    Ok(cols.iter().any(|name| name == column))
}

fn ensure_column(conn: &Connection, table: &str, column: &str, ty: &str) -> Result<()> {
    if !column_exists(conn, table, column)? {
        conn.execute(&format!("ALTER TABLE {table} ADD COLUMN {column} {ty}"), [])?;
    }
    Ok(())
}

fn backfill_workflow_run_reservations(conn: &Connection) -> Result<()> {
    let mut stmt = conn.prepare(
        "SELECT r.id, wr.document FROM workflow_runs r
         JOIN workflow_revisions wr ON wr.workflow_id = r.workflow_id AND wr.revision = r.revision",
    )?;
    let rows = stmt
        .query_map([], |r| Ok((r.get::<_, String>(0)?, r.get::<_, String>(1)?)))?
        .collect::<rusqlite::Result<Vec<_>>>()?;
    for (run_id, document) in rows {
        let doc: crate::workflow::WorkflowDoc = serde_json::from_str(&document)
            .with_context(|| format!("decode pinned workflow document for legacy run {run_id}"))?;
        let estimate = doc.policies.conservative_cost_estimate();
        let estimate = validate_cost_value(estimate, "legacy workflow reservation")?;
        if estimate <= 0.0 {
            bail!("legacy workflow run {run_id} has a non-positive pinned reservation");
        }
        conn.execute(
            "UPDATE workflow_runs SET estimated_cost_usd = ?2 WHERE id = ?1",
            params![run_id, estimate],
        )?;
    }
    Ok(())
}

pub(crate) fn validate_cost_value(value: f64, field: &str) -> Result<f64> {
    if !value.is_finite() || value < 0.0 {
        bail!("{field} must be finite and non-negative, got {value}");
    }
    Ok(value)
}

fn ledger_schema_version(conn: &Connection) -> Result<i64> {
    Ok(conn.query_row("PRAGMA user_version", [], |r| r.get(0))?)
}

fn validate_external_create(input: &ExternalRunCreate) -> Result<()> {
    required_external_field("agent", &input.agent)?;
    required_external_field("role", &input.role)?;
    required_external_field("repo", &input.repo)?;
    required_external_field("brief_hash", &input.brief_hash)?;
    // bitterblossom-922: `plane` on an external run is a descriptive LABEL --
    // which logical/campaign plane the externally-owned run belongs to (e.g.
    // "campaign-2026-07-07-focus") -- not a substrate lease. External runs
    // never lease, dispatch, or execute through the plane (that is the native
    // `runs` table), and `source:"external"` is the discriminator, so an
    // arbitrary label here cannot be confused with a bb-dispatched run. It
    // must be present; it need not be "local". The prior "must be 'local'"
    // check rejected the documented campaign plane value and blocked campaign
    // lanes from registering at all -- pure friction guarding nothing.
    required_external_field("plane", &input.plane)?;
    validate_rfc3339("started_at", input.started_at.trim())?;
    Ok(())
}

fn required_external_field(name: &str, value: &str) -> Result<()> {
    if value.trim().is_empty() {
        bail!("external run field '{name}' is required");
    }
    Ok(())
}

fn validate_external_status(status: &str) -> Result<()> {
    if !EXTERNAL_RUN_STATUSES.contains(&status) {
        bail!("unknown external run status {status}");
    }
    Ok(())
}

fn external_transition_allowed(from: &str, to: &str) -> bool {
    matches!(
        (from, to),
        ("running", "running")
            | ("running", "done")
            | ("running", "failed")
            | ("done", "done")
            | ("failed", "failed")
    )
}

fn validate_rfc3339(field: &str, value: &str) -> Result<()> {
    OffsetDateTime::parse(value, &Rfc3339).with_context(|| format!("{field} must be RFC3339"))?;
    Ok(())
}

fn option_trimmed(value: Option<&str>) -> Option<String> {
    value
        .map(str::trim)
        .filter(|s| !s.is_empty())
        .map(ToString::to_string)
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

#[derive(Clone, Default)]
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
