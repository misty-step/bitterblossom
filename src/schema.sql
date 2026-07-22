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
CREATE INDEX IF NOT EXISTS runs_pending_created
  ON runs(state, created_at, id);

CREATE TABLE IF NOT EXISTS external_runs (
  id TEXT PRIMARY KEY,
  agent TEXT NOT NULL,
  role TEXT NOT NULL,
  repo TEXT NOT NULL,
  brief_hash TEXT NOT NULL,
  plane TEXT NOT NULL,
  status TEXT NOT NULL,
  status_url TEXT,
  receipt_path TEXT,
  started_at TEXT NOT NULL,
  completed_at TEXT,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);

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

-- Public, top-level attempt artifacts must remain inspectable when a hosted
-- runtime loses its ephemeral artifact directories. Bodies are present for
-- every bounded regular file, including binary receipts; oversized files
-- retain metadata so list/read keep their refusal semantics without bloating
-- the ledger.
CREATE TABLE IF NOT EXISTS artifact_snapshots (
  attempt_id INTEGER NOT NULL REFERENCES attempts(id) ON DELETE CASCADE,
  path TEXT NOT NULL,
  size INTEGER NOT NULL,
  content_type TEXT NOT NULL,
  binary INTEGER NOT NULL,
  content BLOB,
  PRIMARY KEY (attempt_id, path)
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
CREATE INDEX IF NOT EXISTS dead_letters_attention
  ON dead_letters(acknowledged_at, created_at, id);

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
  head_version TEXT,
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

-- Backlog 088: a required gate member (e.g. the Thermo-Nuclear maintainability
-- lens) can be explicitly waived for one specific rev of a change by an
-- operator/agent-recorded, risk-tier-tagged reason, instead of hanging the
-- gate pending forever on a diff the tier rule says never needs that member.
-- Scoped by (change_key, rev, kind), not just (change_key, kind): a later rev
-- of the same change is a different diff and needs its own waiver.
CREATE TABLE IF NOT EXISTS member_waivers (
  change_key TEXT NOT NULL,
  rev TEXT NOT NULL,
  kind TEXT NOT NULL,
  reason TEXT NOT NULL,
  created_at TEXT NOT NULL,
  PRIMARY KEY (change_key, rev, kind)
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
  -- Backlog 109: bounded, secret-scrubbed webhook response visibility while a
  -- delivery is retrying, so the actual status/response is debuggable before
  -- the retry budget exhausts it into a final `failed`/discard state.
  last_status_code INTEGER,
  last_response TEXT,
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

-- bitterblossom-workflow-store: revisioned workflow configuration. The
-- database is authoritative for active workflow config; every edit is an
-- immutable revision row (never updated, never deleted), activation selects
-- one revision, and rollback re-activates an old snapshot as a NEW revision.
-- Declarative files are import/export interchange, not a second authority.
CREATE TABLE IF NOT EXISTS workflows (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL UNIQUE,
  state TEXT NOT NULL,             -- draft | active | paused | archived
  active_revision INTEGER,         -- NULL until first activation
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS workflow_revisions (
  workflow_id TEXT NOT NULL REFERENCES workflows(id),
  revision INTEGER NOT NULL,       -- dense, monotonic per workflow
  document TEXT NOT NULL,          -- canonical JSON; immutable
  source TEXT NOT NULL,            -- cli | http | import | import-task | rollback
  note TEXT,
  created_at TEXT NOT NULL,
  PRIMARY KEY (workflow_id, revision)
);

-- Audit trail: every lifecycle act (created/revised/activated/paused/
-- resumed/archived/rolled_back) plus run acceptance and paused-workflow
-- event suppression dispositions.
-- Immutable, activation-time launch materialization. Runtime reads this as
-- evidence; workflow composition owns the snapshot contents and digest.
CREATE TABLE IF NOT EXISTS workflow_step_launch_snapshots (
  workflow_id TEXT NOT NULL,
  revision INTEGER NOT NULL,
  step TEXT NOT NULL,
  snapshot_json TEXT NOT NULL,
  digest TEXT NOT NULL,
  created_at TEXT NOT NULL,
  PRIMARY KEY (workflow_id, revision, step),
  FOREIGN KEY (workflow_id, revision)
    REFERENCES workflow_revisions(workflow_id, revision)
);
CREATE INDEX IF NOT EXISTS workflow_step_launch_snapshots_revision
  ON workflow_step_launch_snapshots(workflow_id, revision);

CREATE TABLE IF NOT EXISTS workflow_events (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  workflow_id TEXT NOT NULL REFERENCES workflows(id),
  run_id TEXT REFERENCES workflow_runs(id),
  kind TEXT NOT NULL,
  data TEXT,
  at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS workflow_events_workflow
  ON workflow_events(workflow_id);

-- One accepted workflow run pins the revision active at acceptance. New
-- activations affect new events only; this row never changes revision.
-- `dedupe_key` (bitterblossom-workflow-runtime-v1) is the normalized
-- acceptance contract's idempotency handle: every trigger source (external
-- webhook, schedule, internal, synthetic test) derives one, and a repeat
-- acceptance returns the original run as a duplicate instead of forking.
CREATE TABLE IF NOT EXISTS workflow_runs (
  id TEXT PRIMARY KEY,
  workflow_id TEXT NOT NULL REFERENCES workflows(id),
  revision INTEGER NOT NULL,
  trigger_kind TEXT NOT NULL,
  payload TEXT,
  dedupe_key TEXT,
  estimated_cost_usd REAL NOT NULL DEFAULT 1.0,
  created_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS workflow_runs_workflow
  ON workflow_runs(workflow_id);

-- bitterblossom-workflow-runtime-v1: runtime state for accepted workflow
-- runs. Kept in a sibling table so acceptance rows stay immutable (the
-- store's audited invariant); this row is the mutable execution status of
-- one run group. States: queued | running | succeeded | failed |
-- incomplete | stopped | needs_attention.
CREATE TABLE IF NOT EXISTS workflow_run_status (
  run_id TEXT PRIMARY KEY REFERENCES workflow_runs(id),
  state TEXT NOT NULL,
  detail TEXT,                     -- guard name, error, or exact uncertainty
  current_step TEXT,
  stop_requested INTEGER NOT NULL DEFAULT 0,
  stop_reason TEXT,
  cost_usd REAL,                   -- sum of OBSERVED step costs; NULL = none reported
  started_at TEXT,
  updated_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS workflow_run_status_queue
  ON workflow_run_status(state, updated_at, run_id);

-- Every step attempt in a run group, in one dense sequence. `agent_json`
-- is the pinned StepAgent snapshot that actually launched; `authority_json`
-- is the effective grant labels the step ran under.
CREATE TABLE IF NOT EXISTS workflow_step_runs (
  id TEXT PRIMARY KEY,
  run_id TEXT NOT NULL REFERENCES workflow_runs(id),
  step TEXT NOT NULL,
  attempt INTEGER NOT NULL,        -- 1-based, dense per run group
  agent_json TEXT NOT NULL,
  goal TEXT NOT NULL,
  state TEXT NOT NULL,             -- running | succeeded | failed | incomplete
  outcome TEXT,                    -- declared completion outcome (branching steps)
  summary TEXT,
  error TEXT,
  exit_code INTEGER,
  tokens_in INTEGER,
  tokens_out INTEGER,
  turns INTEGER,
  cost_usd REAL,
  artifact_dir TEXT,
  authority_json TEXT NOT NULL,
  started_at TEXT NOT NULL,
  ended_at TEXT,
  UNIQUE (run_id, attempt)
);
CREATE INDEX IF NOT EXISTS workflow_step_runs_run
  ON workflow_step_runs(run_id);
CREATE INDEX IF NOT EXISTS workflow_step_runs_state
  ON workflow_step_runs(run_id, state, attempt);

-- Dynamic child agents an executing step declared (CHILD_AGENTS.json).
-- Evidence rows under the parent step run only — children never become
-- workflow or agent catalog entries. `inherited` = 1 when the child took
-- the parent grant verbatim; a declared grant must be a subset.
CREATE TABLE IF NOT EXISTS workflow_child_agents (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  step_run_id TEXT NOT NULL REFERENCES workflow_step_runs(id),
  name TEXT NOT NULL,
  harness TEXT,
  model TEXT,
  goal TEXT,
  authority_json TEXT NOT NULL,
  inherited INTEGER NOT NULL,
  cost_usd REAL,
  result TEXT,
  recorded_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS workflow_child_agents_step
  ON workflow_child_agents(step_run_id);

-- bitterblossom-930: HITL asks (question|decision|approval) a dispatched
-- attempt raises via the `bb ask` CLI. `state`: open -> answered (fast path,
-- the still-running attempt polls and sees it) or open -> parked (window
-- elapsed with no answer; the run finalizes as `parked_on_ask` and answering
-- a parked ask creates a lineage-linked resume run instead of unblocking a
-- poll). `blocking` distinguishes park+escalate from proceed-on-assumption.
CREATE TABLE IF NOT EXISTS asks (
  id TEXT PRIMARY KEY,
  run_id TEXT NOT NULL REFERENCES runs(id),
  task TEXT NOT NULL,
  kind TEXT NOT NULL,
  question TEXT NOT NULL,
  context TEXT,
  blocking INTEGER NOT NULL,
  window_seconds INTEGER NOT NULL,
  state TEXT NOT NULL,
  answer TEXT,
  answered_at TEXT,
  answered_by TEXT,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS asks_run ON asks(run_id);
