#!/bin/sh
# Production-operations smoke drill. Default local mode is deterministic and
# runs in CI; remote mode performs read-only probes against the Fly plane.
set -eu

cd "$(dirname "$0")/.."

MODE=local
BB_BIN=${BB_BIN:-./target/debug/bb}
BB_URL=${BB_URL:-https://bitterblossom-plane-9xpa5.ondigitalocean.app}
BB_FLY_APP=${BB_FLY_APP:-bitterblossom-plane}
TMP=
SERVER_PID=
SERVER_LOG=
PORT=
TOKEN=ops-drill-token

usage() {
  cat <<'USAGE'
usage: scripts/production-ops-drill.sh [--local|--remote] [--bb-bin PATH] [--url URL] [--fly-app APP]

Local mode creates a temporary dev plane, runs one local workload, starts
`bb serve`, probes /health and authenticated read APIs, runs `bb recover`, and
proves the SQLite ledger can be backed up and restored without losing run
history.

Remote mode probes the Fly plane without sending tokens in query strings:
  BB_API_TOKEN=... scripts/production-ops-drill.sh --remote

Remote mode additionally runs `flyctl status`, `flyctl volumes list`, and
`BB_PLANE_DIR=${BB_PLANE_DIR:-/app/plane} bb recover --json` inside the Fly machine when `flyctl` is
available and authenticated.
USAGE
}

need() {
  command -v "$1" >/dev/null 2>&1 || {
    echo "ops drill: missing required command: $1" >&2
    exit 2
  }
}

while [ "$#" -gt 0 ]; do
  case "$1" in
    --local)
      MODE=local
      shift
      ;;
    --remote)
      MODE=remote
      shift
      ;;
    --bb-bin)
      [ "$#" -ge 2 ] || { echo "ops drill: --bb-bin needs a path" >&2; exit 2; }
      BB_BIN=$2
      shift 2
      ;;
    --url)
      [ "$#" -ge 2 ] || { echo "ops drill: --url needs a value" >&2; exit 2; }
      BB_URL=${2%/}
      shift 2
      ;;
    --fly-app)
      [ "$#" -ge 2 ] || { echo "ops drill: --fly-app needs a value" >&2; exit 2; }
      BB_FLY_APP=$2
      shift 2
      ;;
    -h | --help)
      usage
      exit 0
      ;;
    *)
      echo "ops drill: unknown argument: $1" >&2
      usage >&2
      exit 2
      ;;
  esac
done

need curl
need python3

cleanup() {
  if [ -n "$SERVER_PID" ]; then
    kill "$SERVER_PID" 2>/dev/null || true
    wait "$SERVER_PID" 2>/dev/null || true
  fi
  if [ -n "$TMP" ]; then
    if [ "${BB_OPS_DRILL_KEEP_TMP:-0}" = "1" ]; then
      echo "kept temp plane: $TMP"
    else
      rm -rf "$TMP"
    fi
  fi
}
trap cleanup EXIT INT TERM

free_port() {
  python3 - <<'PY'
import socket
s = socket.socket()
s.bind(("127.0.0.1", 0))
print(s.getsockname()[1])
s.close()
PY
}

expect_code() {
  label=$1
  expected=$2
  shift 2
  out="$TMP/response-$label.txt"
  code=$(curl -sS -o "$out" -w "%{http_code}" "$@")
  if [ "$code" != "$expected" ]; then
    echo "FAIL $label: expected HTTP $expected, got $code" >&2
    cat "$out" >&2 || true
    [ -z "$SERVER_LOG" ] || { echo "serve log:" >&2; cat "$SERVER_LOG" >&2 || true; }
    exit 1
  fi
  echo "ok:$label http=$code"
}

expect_bearer_code() {
  label=$1
  expected=$2
  token=$3
  url=$4
  out="$TMP/response-$label.txt"
  code=$(
    {
      printf '%s\n' 'silent'
      printf '%s\n' 'show-error'
      printf 'url = "%s"\n' "$url"
      printf 'header = "Authorization: Bearer %s"\n' "$token"
    } | curl --config - -o "$out" -w "%{http_code}"
  )
  if [ "$code" != "$expected" ]; then
    echo "FAIL $label: expected HTTP $expected, got $code" >&2
    cat "$out" >&2 || true
    [ -z "$SERVER_LOG" ] || { echo "serve log:" >&2; cat "$SERVER_LOG" >&2 || true; }
    exit 1
  fi
  echo "ok:$label http=$code"
}

write_local_plane() {
  mkdir -p "$TMP/agents" "$TMP/tasks/smoke" "$TMP/tasks/verify"
  cat >"$TMP/plane.toml" <<'EOF'
dev = true

[ingress]
bind = "127.0.0.1:7077"

[backup]
enabled = true
provider = "litestream"
replica_env = "LITESTREAM_REPLICA_URL"
last_success_path = ".bb/backup-last-success"
rpo_seconds = 300
rto_seconds = 1800

[gate]
required = ["verify"]
quorum = 1
EOF
  cat >"$TMP/agents/smoke.toml" <<EOF
version = 1
harness = "command"
bin = "$TMP/smoke-command.sh"
EOF
  cat >"$TMP/smoke-command.sh" <<'EOF'
#!/bin/sh
cat >/dev/null
printf '%s\n' "ops smoke ok"
EOF
  chmod +x "$TMP/smoke-command.sh"
  cat >"$TMP/tasks/smoke/card.md" <<'EOF'
# Operations smoke

Return a successful command result. This task exists only for the local
operations drill.
EOF
  cat >"$TMP/tasks/smoke/task.toml" <<'EOF'
agent = "smoke"
substrate = "local"

[budget]
timeout_minutes = 1
max_cost_per_run_usd = 0.01

[[trigger]]
kind = "manual"
EOF
  cat >"$TMP/tasks/verify/card.md" <<'EOF'
# Operations gate fixture

This task is not dispatched by the local drill. It exists so restored ledgers
can prove the read-only gate surface still evaluates against submission rows.
EOF
  cat >"$TMP/tasks/verify/task.toml" <<'EOF'
agent = "smoke"
substrate = "local"
verdict = "verify"

[[trigger]]
kind = "manual"
EOF
}

wait_for_server() {
  i=0
  while [ "$i" -lt 100 ]; do
    if ! kill -0 "$SERVER_PID" 2>/dev/null; then
      echo "bb serve exited early; log follows" >&2
      cat "$SERVER_LOG" >&2 || true
      exit 1
    fi
    code=$(curl -sS -o /dev/null -w "%{http_code}" "http://127.0.0.1:$PORT/health" 2>/dev/null || true)
    [ "$code" = "200" ] && return
    i=$((i + 1))
    sleep 0.1
  done
  echo "bb serve did not become healthy on port $PORT; log follows" >&2
  cat "$SERVER_LOG" >&2 || true
  exit 1
}

backup_restore_check() {
  db=$1
  backup=$2
  restored=$3
  python3 - "$db" "$backup" "$restored" <<'PY'
import json
import sqlite3
import sys

db, backup, restored = sys.argv[1:]
src = sqlite3.connect(db)
dst = sqlite3.connect(backup)
src.backup(dst)
dst.close()
src.close()

check = sqlite3.connect(backup)
integrity = check.execute("pragma integrity_check").fetchone()[0]
run_count = check.execute("select count(*) from runs").fetchone()[0]
check.close()

restore = sqlite3.connect(restored)
backup_conn = sqlite3.connect(backup)
backup_conn.backup(restore)
backup_conn.close()
restored_count = restore.execute("select count(*) from runs").fetchone()[0]
restore.close()

if integrity != "ok":
    raise SystemExit(f"integrity_check failed: {integrity}")
if run_count < 1:
    raise SystemExit("backup did not contain run history")
if restored_count != run_count:
    raise SystemExit(f"restored run count {restored_count} != backup run count {run_count}")
print(json.dumps({"integrity": integrity, "runs": run_count, "restored_runs": restored_count}))
PY
}

restore_read_surface_check() {
  restored_plane=$1
  mkdir -p "$restored_plane"
  cp -R "$TMP/agents" "$TMP/tasks" "$restored_plane/"
  cat >"$restored_plane/plane.toml" <<'EOF'
dev = true
db_path = "../restored.db"

[gate]
required = ["verify"]
quorum = 1
EOF
  "$BB_BIN" --config "$restored_plane" check >/dev/null
  "$BB_BIN" --config "$restored_plane" status --json >/dev/null
  "$BB_BIN" --config "$restored_plane" runs list --json >/dev/null
  "$BB_BIN" --config "$restored_plane" gate --change ops-drill --json >/dev/null
  echo "ok:restore-read-surfaces check status runs gate"
}

run_local() {
  [ -x "$BB_BIN" ] || {
    echo "ops drill: bb binary not found at $BB_BIN; run cargo build or set --bb-bin" >&2
    exit 2
  }
  TMP=$(mktemp -d "${TMPDIR:-/tmp}/bb-ops-drill.XXXXXX")
  write_local_plane
  "$BB_BIN" --config "$TMP" check >/dev/null
  "$BB_BIN" --config "$TMP" run smoke --payload '{"drill":"operations"}' --json >"$TMP/run.json"
  "$BB_BIN" --config "$TMP" submit open --change ops-drill --rev local --json >"$TMP/submission.json"
  "$BB_BIN" --config "$TMP" gate --change ops-drill --json >"$TMP/gate.json"
  "$BB_BIN" --config "$TMP" recover --json >"$TMP/recover.json"
  python3 - "$TMP/.bb/backup-last-success" <<'PY'
import datetime
import pathlib
import sys

pathlib.Path(sys.argv[1]).write_text(
    datetime.datetime.now(datetime.timezone.utc).isoformat().replace("+00:00", "Z")
)
PY

  PORT=$(free_port)
  SERVER_LOG="$TMP/serve.log"
  BB_INGRESS_BIND="127.0.0.1:$PORT" BB_API_TOKEN="$TOKEN" \
    "$BB_BIN" --config "$TMP" serve >"$SERVER_LOG" 2>&1 &
  SERVER_PID=$!
  wait_for_server

  expect_code local-health 200 "http://127.0.0.1:$PORT/health"
  expect_code local-tasks-noauth 401 "http://127.0.0.1:$PORT/api/tasks"
  expect_bearer_code local-tasks 200 "$TOKEN" "http://127.0.0.1:$PORT/api/tasks"
  expect_bearer_code local-runs 200 "$TOKEN" "http://127.0.0.1:$PORT/api/runs"
  expect_bearer_code local-status 200 "$TOKEN" "http://127.0.0.1:$PORT/api/status"
  python3 - "$TMP/response-local-status.txt" <<'PY'
import json
import pathlib
import sys

doc = json.loads(pathlib.Path(sys.argv[1]).read_text())
backup = doc.get("backup", {})
if backup.get("status") != "fresh" or backup.get("healthy") is not True:
    raise SystemExit(f"backup status was not fresh: {backup}")
if backup.get("replica_env") != "LITESTREAM_REPLICA_URL":
    raise SystemExit(f"backup replica env leaked or drifted: {backup}")
PY
  expect_bearer_code local-html 200 "$TOKEN" "http://127.0.0.1:$PORT/"

  backup_restore_check "$TMP/.bb/plane.db" "$TMP/plane.db.backup" "$TMP/restored.db" \
    | sed 's/^/backup_restore: /'
  restore_read_surface_check "$TMP/restored-plane"
  echo "ops drill: local pass"
}

run_remote() {
  [ -n "${BB_API_TOKEN:-}" ] || {
    echo "ops drill: BB_API_TOKEN is required for --remote" >&2
    exit 2
  }
  TMP=$(mktemp -d "${TMPDIR:-/tmp}/bb-ops-remote.XXXXXX")
  expect_code remote-health 200 "$BB_URL/health"
  expect_bearer_code remote-tasks 200 "$BB_API_TOKEN" "$BB_URL/api/tasks"
  expect_bearer_code remote-runs 200 "$BB_API_TOKEN" "$BB_URL/api/runs"
  expect_bearer_code remote-status 200 "$BB_API_TOKEN" "$BB_URL/api/status"

  if command -v flyctl >/dev/null 2>&1; then
    flyctl status --app "$BB_FLY_APP" >/dev/null
    flyctl volumes list --app "$BB_FLY_APP" >/dev/null
    flyctl ssh console --app "$BB_FLY_APP" --command '/bin/sh -lc "BB_PLANE_DIR=${BB_PLANE_DIR:-/app/plane} bb recover --json"' >"$TMP/recover.json"
    echo "ok:remote-fly status volumes recover"
  else
    echo "skip:remote-fly flyctl not found"
  fi
  echo "ops drill: remote pass"
}

case "$MODE" in
  local) run_local ;;
  remote) run_remote ;;
esac
