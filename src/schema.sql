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
  config_source_repo TEXT,
  config_source_ref TEXT,
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
  replayed_run_id TEXT,
  acknowledged_reason TEXT,
  acknowledged_at TEXT
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
  UNIQUE (submission_id, kind, run_id)
);

CREATE TABLE IF NOT EXISTS rejections (
  change_key TEXT NOT NULL,
  fingerprint TEXT NOT NULL,
  reason TEXT NOT NULL,
  created_at TEXT NOT NULL,
  PRIMARY KEY (change_key, fingerprint)
);

-- Backlog 083: unattended-loop guardrails. Guard events are the durable,
-- operator-visible surface for circuit breakers: ingress body rejections,
-- cron catch-up collapses, notification failures, and plane pause/resume.
-- `count` lets a collapse record how many fires it skipped in one row.
CREATE TABLE IF NOT EXISTS guard_events (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  kind TEXT NOT NULL,
  task TEXT,
  detail TEXT,
  count INTEGER NOT NULL DEFAULT 1,
  at TEXT NOT NULL
);

-- Backlog 089: outbound notifications are durable before transport. This is
-- the operator-visible outbox behind the webhook notifier; pending rows mean
-- delivery was queued but not observed, failed rows are retry/ack candidates.
CREATE TABLE IF NOT EXISTS notification_outbox (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  event TEXT NOT NULL,
  payload TEXT NOT NULL,
  status TEXT NOT NULL,
  attempts INTEGER NOT NULL DEFAULT 0,
  last_error TEXT,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  delivered_at TEXT,
  acknowledged_reason TEXT,
  acknowledged_at TEXT
);

-- Single-row table: presence of row 1 means reflex dispatch is paused.
-- Distinct from per-task parking (parked_tasks): a pause halts the
-- autonomous dispatch loop for the whole plane, not one task's budget.
CREATE TABLE IF NOT EXISTS plane_pause (
  row INTEGER PRIMARY KEY CHECK (row = 1),
  reason TEXT NOT NULL,
  at TEXT NOT NULL
);
