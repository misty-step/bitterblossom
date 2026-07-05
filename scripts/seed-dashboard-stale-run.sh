#!/bin/sh
# bitterblossom-119: adds one run frozen in 'running' state with a
# 40-minute-old created_at (past PROGRESS_STALE_SECONDS=1800s) and no
# progress marker, so progress::classify reports stale_executing --
# rendering the dashboard's "stale" freshness badge on the Runs view.
#
# Must run AFTER `bb serve` is already listening against this plane dir --
# see seed-dashboard-fixture.sh for why a pre-boot 'running' row gets swept
# by boot-time recovery instead of staying stale.
#
# Usage: scripts/seed-dashboard-stale-run.sh <plane-dir>
set -eu

PLANE_DIR="${1:?usage: seed-dashboard-stale-run.sh <plane-dir>}"
DB="$PLANE_DIR/.bb/plane.db"
STALE_AT="$(date -u -v-40M +%Y-%m-%dT%H:%M:%SZ 2>/dev/null || date -u -d '40 minutes ago' +%Y-%m-%dT%H:%M:%SZ)"

sqlite3 "$DB" <<SQL
INSERT INTO runs (id, task, trigger_kind, state, trace_id, agent_name, agent_version, created_at, updated_at)
VALUES ('run-stale-demo', 'demo', 'manual', 'running', 'trace-stale-demo', 'stub', 1, '$STALE_AT', '$STALE_AT');

INSERT INTO host_leases (host, run_id, acquired_at)
VALUES ('screenshot-host', 'run-stale-demo', '$STALE_AT');
SQL

echo "inserted stale run-stale-demo into $DB (run bb serve's already-open API to confirm classification)"
