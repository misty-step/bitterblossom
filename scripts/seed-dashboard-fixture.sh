#!/bin/sh
# bitterblossom-119: seeds a plane directory with realistic data across most
# dashboard data sources -- completed runs, a DLQ entry, a failed
# notification, a blocked submission with verdicts, and an open task list --
# so `bb serve` against it renders the dashboard's "populated" state for the
# rendered-screenshot proof under docs/screenshots/operator-dashboard/.
# Empty/auth-required/error states need no fixture (an empty plane, or the
# plane down, is the fixture).
#
# The "stale" state (a run/lease frozen at 'running') is deliberately NOT
# seeded here: `bb serve`'s boot-time recovery sweep
# (recovery::recover_inherited_runs) correctly treats any 'running' row with
# no live process behind it as inherited-from-a-dead-plane and reclassifies
# it before the HTTP listener even opens -- exactly the safety behavior a
# real stuck run should get. Seeding it pre-boot just gets it swept into a
# dead letter. Run seed-dashboard-stale-run.sh AFTER `bb serve` is already up
# to add it live instead.
#
# Usage: scripts/seed-dashboard-fixture.sh <plane-dir>
# Then:  BB_API_TOKEN=demo-token BB_INGRESS_BIND=127.0.0.1:PORT \
#          target/debug/bb --config <plane-dir> serve
# Then:  scripts/seed-dashboard-stale-run.sh <plane-dir>   # for the stale state
set -eu

PLANE_DIR="${1:?usage: seed-dashboard-fixture.sh <plane-dir>}"
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
BB="${BB_BIN:-$ROOT/target/debug/bb}"

mkdir -p "$PLANE_DIR/agents" "$PLANE_DIR/tasks/demo"

cat > "$PLANE_DIR/plane.toml" <<'EOF'
dev = true
[ingress]
bind = "127.0.0.1:0"
[gate]
required = ["reviewer"]
EOF

stub="$PLANE_DIR/stub.sh"
printf '#!/bin/sh\ncat >/dev/null\necho ok\n' > "$stub"
chmod +x "$stub"
cat > "$PLANE_DIR/agents/stub.toml" <<EOF
harness = "command"
model = ""
bin = "$stub"
EOF

echo "demo task" > "$PLANE_DIR/tasks/demo/card.md"
cat > "$PLANE_DIR/tasks/demo/task.toml" <<'EOF'
agent = "stub"
substrate = "local"
[[trigger]]
kind = "manual"
[[trigger]]
kind = "cron"
schedule = "0 * * * *"
[budget]
max_runs_per_day = 20
max_cost_per_run_usd = 0.5
timeout_minutes = 10
EOF

# A second, never-dispatched task purely so [gate] required=["reviewer"] has
# a task declaring that verdict kind (plane validation requires it); the
# submission/verdict rows below are seeded directly rather than by actually
# running this task.
mkdir -p "$PLANE_DIR/tasks/review"
echo "review task" > "$PLANE_DIR/tasks/review/card.md"
cat > "$PLANE_DIR/tasks/review/task.toml" <<'EOF'
agent = "stub"
substrate = "local"
verdict = "reviewer"
[[trigger]]
kind = "manual"
EOF

# Two real completed runs via the actual dispatch path.
"$BB" --config "$PLANE_DIR" run demo --json >/dev/null
"$BB" --config "$PLANE_DIR" run demo --json >/dev/null

DB="$PLANE_DIR/.bb/plane.db"
NOW="$(date -u +%Y-%m-%dT%H:%M:%SZ)"

sqlite3 "$DB" <<SQL
INSERT INTO dead_letters (run_id, task, payload, error, created_at)
VALUES ('run-dlq-demo', 'demo', '{}', 'agent exited 1: missing OPENROUTER_API_KEY', '$NOW');

INSERT INTO notification_outbox (event, payload, status, attempts, last_error, created_at, updated_at)
VALUES ('gate_blocked', '{"submission":"sub-demo-1"}', 'failed', 3, 'webhook returned 503', '$NOW', '$NOW');

INSERT INTO submissions (id, change_key, rev, round, state, created_at, updated_at)
VALUES ('sub-demo-1', 'refs/pull/119', 'deadbeef00000000000000000000000000000001', 1, 'blocked', '$NOW', '$NOW');

INSERT INTO verdicts (submission_id, run_id, kind, verdict, findings_json, created_at)
VALUES ('sub-demo-1', 'run-verdict-1', 'reviewer', 'blocking', '[{"severity":"blocking","claim":"missing test coverage"}]', '$NOW');
SQL

echo "seeded plane at $PLANE_DIR (db: $DB)"
