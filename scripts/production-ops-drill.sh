#!/bin/sh
# Local-primary production operations drill.
# --primary is read-only against the configured launchd plane; --dev-temp is a
# throwaway fixture that exercises the same readiness, drain, and restore path.
set -eu

cd "$(dirname "$0")/.."

MODE=primary
BB_CONFIG_ARG=
BB_BIN_ARG=

usage() {
  cat <<'USAGE'
usage: scripts/production-ops-drill.sh [--primary|--dev-temp] [--config PATH] [--bb-bin PATH]

Primary mode (the default) is a non-destructive readback of the configured
launchd service: plane.toml, /health, status/runs/DLQ JSON, SQLite WAL/integrity,
backup heartbeat, launchd ownership, and resource headroom. It never runs,
replays, acknowledges, drains, restarts, or edits the live ledger. A status=open
DLQ is reported as readiness BLOCKED and returns exit 3.

Set BB_ENV_FILE to an operator-local env file when the API requires a bearer
credential. The file is sourced silently (without xtrace), never printed, and
must be readable only by its owner (0600 or stricter). When no token is
configured, --primary accepts unauthenticated API reads only for a loopback
bind; a configured token proves both the unauthenticated 401 boundary and the
authenticated bearer 200 path. --dev-temp always uses its fixture token.

Dev-temp mode creates an isolated dev=true fixture, dispatches one smoke run,
proves authenticated read APIs, sends SIGTERM and restarts the fixture, then
backs up/restores SQLite into a separate directory. It is not production proof.
USAGE
}

while [ "$#" -gt 0 ]; do
  case "$1" in
    --primary) MODE=primary; shift ;;
    --dev-temp) MODE=dev-temp; shift ;;
    --config)
      [ "$#" -ge 2 ] || { echo "ops drill: --config needs a path" >&2; exit 2; }
      BB_CONFIG_ARG=$2
      shift 2
      ;;
    --bb-bin)
      [ "$#" -ge 2 ] || { echo "ops drill: --bb-bin needs a path" >&2; exit 2; }
      BB_BIN_ARG=$2
      shift 2
      ;;
    -h|--help) usage; exit 0 ;;
    *) echo "ops drill: unknown argument: $1" >&2; usage >&2; exit 2 ;;
  esac
done

BB_CONFIG="${BB_CONFIG_ARG:-$(printenv BB_RUNTIME_PLANE 2>/dev/null || printf '%s' plane)}"
. "$(pwd)/scripts/bb-operator-env.sh"
bb_source_operator_env "$(pwd)" || { echo "ops drill: failed to load operator env" >&2; exit 2; }

BB_BIN="${BB_BIN_ARG:-$(printenv BB_BIN 2>/dev/null || printf '%s' ./target/debug/bb)}"
TMP=
SERVER_PID=
SERVER_LOG=
PORT=
TOKEN="$(printenv BB_OPS_DRILL_TOKEN 2>/dev/null || printf '%s' ops-drill-token)"

need() {
  command -v "$1" >/dev/null 2>&1 || {
    echo "ops drill: missing required command: $1" >&2
    exit 2
  }
}

need curl
need python3

cleanup() {
  if [ -n "$SERVER_PID" ]; then
    kill "$SERVER_PID" 2>/dev/null || true
    wait "$SERVER_PID" 2>/dev/null || true
  fi
  if [ -n "$TMP" ]; then
    if [ "$(printenv BB_OPS_DRILL_KEEP_TMP 2>/dev/null || true)" = "1" ]; then
      echo "kept temp plane: $TMP"
    else
      rm -rf "$TMP"
    fi
  fi
}
trap cleanup EXIT INT TERM

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
      printf '%s\n' silent
      printf '%s\n' show-error
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

write_dev_plane() {
  mkdir -p "$TMP/agents" "$TMP/tasks/smoke" "$TMP/tasks/verify"
  cat >"$TMP/plane.toml" <<'EOF'
dev = true
allow_local_substrate = true

[ingress]
bind = "127.0.0.1:0"

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

Return a successful command result. This task exists only for the isolated
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

This task exists only so the restored ledger can evaluate the read-only gate
surface against submission rows.
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
import pathlib
import sqlite3
import sys

db, backup, restored = sys.argv[1:]
src = sqlite3.connect(db)
mode = src.execute("pragma journal_mode").fetchone()[0]
integrity = src.execute("pragma integrity_check").fetchone()[0]
run_count = src.execute("select count(*) from runs").fetchone()[0]
src.backup(sqlite3.connect(backup))
src.close()

check = sqlite3.connect(backup)
backup_integrity = check.execute("pragma integrity_check").fetchone()[0]
backup_runs = check.execute("select count(*) from runs").fetchone()[0]
check.close()

restore = sqlite3.connect(restored)
backup_conn = sqlite3.connect(backup)
backup_conn.backup(restore)
backup_conn.close()
restored_integrity = restore.execute("pragma integrity_check").fetchone()[0]
restored_count = restore.execute("select count(*) from runs").fetchone()[0]
restore.close()

if mode.lower() != "wal":
    raise SystemExit(f"source journal mode was not WAL: {mode}")
if integrity != "ok" or backup_integrity != "ok" or restored_integrity != "ok":
    raise SystemExit(f"integrity check failed: source={integrity} backup={backup_integrity} restored={restored_integrity}")
if run_count < 1 or backup_runs != run_count or restored_count != run_count:
    raise SystemExit(f"run history changed: source={run_count} backup={backup_runs} restored={restored_count}")
print(json.dumps({"journal_mode": mode, "integrity": integrity, "backup_integrity": backup_integrity, "restored_integrity": restored_integrity, "runs": run_count, "restored_runs": restored_count}, sort_keys=True))
PY
}

restore_read_surface_check() {
  restored_plane=$1
  mkdir -p "$restored_plane"
  cp -R "$TMP/agents" "$TMP/tasks" "$restored_plane/"
  cat >"$restored_plane/plane.toml" <<'EOF'
dev = true
allow_local_substrate = true
db_path = "../restored.db"

[ingress]
bind = "127.0.0.1:0"

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

run_dev_temp() {
  [ -x "$BB_BIN" ] || { echo "ops drill: bb binary not found at $BB_BIN" >&2; exit 2; }
  TMP=$(mktemp -d /tmp/bb-ops-drill.XXXXXX)
  write_dev_plane
  "$BB_BIN" --config "$TMP" check >/dev/null
  "$BB_BIN" --config "$TMP" run smoke --payload '{"drill":"operations"}' --json >"$TMP/run.json"
  "$BB_BIN" --config "$TMP" submit open --change ops-drill --rev dev-temp --json >"$TMP/submission.json"
  "$BB_BIN" --config "$TMP" gate --change ops-drill --json >"$TMP/gate.json"
  python3 - "$TMP/.bb/backup-last-success" <<'PY'
import datetime
import pathlib
import sys
pathlib.Path(sys.argv[1]).write_text(datetime.datetime.now(datetime.timezone.utc).isoformat().replace("+00:00", "Z"))
PY

  PORT=$(python3 - <<'PY'
import socket
s = socket.socket()
s.bind(("127.0.0.1", 0))
print(s.getsockname()[1])
s.close()
PY
)
  SERVER_LOG="$TMP/serve.log"
  BB_INGRESS_BIND="127.0.0.1:$PORT" BB_API_TOKEN="$TOKEN" "$BB_BIN" --config "$TMP" serve >"$SERVER_LOG" 2>&1 &
  SERVER_PID=$!
  wait_for_server
  expect_code dev-temp-health 200 "http://127.0.0.1:$PORT/health"
  expect_code dev-temp-tasks-noauth 401 "http://127.0.0.1:$PORT/api/tasks"
  expect_bearer_code dev-temp-tasks 200 "$TOKEN" "http://127.0.0.1:$PORT/api/tasks"
  expect_bearer_code dev-temp-status 200 "$TOKEN" "http://127.0.0.1:$PORT/api/status"
  backup_restore_check "$TMP/.bb/plane.db" "$TMP/plane.db.backup" "$TMP/restored.db" | sed 's/^/backup_restore: /'

  kill -TERM "$SERVER_PID"
  wait "$SERVER_PID"
  grep -q 'SIGTERM received, shutting down' "$SERVER_LOG"
  echo "ok:dev-temp-sigterm-drain"
  BB_INGRESS_BIND="127.0.0.1:$PORT" BB_API_TOKEN="$TOKEN" "$BB_BIN" --config "$TMP" serve >>"$SERVER_LOG" 2>&1 &
  SERVER_PID=$!
  wait_for_server
  expect_code dev-temp-restarted-health 200 "http://127.0.0.1:$PORT/health"
  "$BB_BIN" --config "$TMP" status --json >"$TMP/restarted-status.json"
  "$BB_BIN" --config "$TMP" recover --json >"$TMP/recover.json"
  restore_read_surface_check "$TMP/restored-plane"
  echo "ops drill: dev-temp pass"
}

read_primary_config() {
  python3 - "$BB_CONFIG" "$TMP/primary-config.json" <<'PY'
import json
import os
import pathlib
import sys
import tomllib

root = pathlib.Path(sys.argv[1]).expanduser().resolve()
with (root / "plane.toml").open("rb") as handle:
    doc = tomllib.load(handle)
raw_db = doc.get("db_path", ".bb/plane.db")
db = pathlib.Path(raw_db)
if not db.is_absolute():
    db = root / db
backup = doc.get("backup", {}) or {}
raw_heartbeat = backup.get("last_success_path", ".bb/backup-last-success")
heartbeat = pathlib.Path(raw_heartbeat)
if not heartbeat.is_absolute():
    heartbeat = root / heartbeat
ingress = doc.get("ingress", {}) or {}
configured_bind = ingress.get("bind")
override = os.environ.get("BB_INGRESS_BIND", "").strip()
if configured_bind and override and configured_bind != override:
    raise SystemExit(
        f"BB_INGRESS_BIND={override!r} disagrees with [ingress].bind={configured_bind!r}"
    )
bind = override or configured_bind or "127.0.0.1:7093"
checkout = root.parent
if root.name != "plane":
    raise SystemExit(f"local-primary plane must be the checkout's plane/ directory: {root}")
result = {
    "root": str(root),
    "checkout": str(checkout),
    "db": str(db.resolve()),
    "heartbeat": str(heartbeat.resolve()),
    "bind": bind,
    "dev": bool(doc.get("dev", False)),
    "allow_local_substrate": bool(doc.get("allow_local_substrate", False)),
    "backup_enabled": bool(backup.get("enabled", False)),
    "replica_env": backup.get("replica_env"),
}
pathlib.Path(sys.argv[2]).write_text(json.dumps(result, sort_keys=True))
print(json.dumps(result, sort_keys=True))
PY
}

read_http_json() {
  label=$1
  url=$2
  out="$TMP/$label.json"
  if [ -n "${BB_API_TOKEN:-}" ]; then
    code=$(
      {
        printf '%s\n' silent
        printf '%s\n' show-error
        printf 'url = "%s"\n' "$url"
        printf 'header = "Authorization: Bearer %s"\n' "$BB_API_TOKEN"
      } | curl --config - -o "$out" -w "%{http_code}"
    )
  else
    code=$(curl -sS -o "$out" -w "%{http_code}" "$url")
  fi
  if [ "$code" != "200" ]; then
    echo "FAIL $label: expected HTTP 200, got $code" >&2
    cat "$out" >&2 || true
    exit 1
  fi
  echo "ok:$label http=$code"
}

primary_api_auth() {
  url=$1
  case "$bind" in
    127.0.0.1:*|localhost:*) ;;
    *)
      echo "FAIL primary API has no token but bind is not loopback: $bind" >&2
      exit 1
      ;;
  esac
  if [ -n "${BB_API_TOKEN:-}" ]; then
    expect_code primary-status-no-auth 401 "$url"
    expect_bearer_code primary-status-auth 200 "$BB_API_TOKEN" "$url"
  else
    expect_code primary-status-loopback-no-token 200 "$url"
  fi
}

sqlite_snapshot() {
  python3 - "$1" "$2" <<'PY'
import hashlib
import json
import pathlib
import sqlite3
import sys
import urllib.parse

path = pathlib.Path(sys.argv[1]).resolve()
out = pathlib.Path(sys.argv[2])
if not path.exists():
    raise SystemExit(f"SQLite ledger missing: {path}")
uri = "file:" + urllib.parse.quote(str(path), safe="/") + "?mode=ro&immutable=0"
conn = sqlite3.connect(uri, uri=True)
conn.execute("PRAGMA query_only = ON")
try:
    journal_mode = conn.execute("PRAGMA journal_mode").fetchone()[0]
    integrity = conn.execute("PRAGMA integrity_check").fetchone()[0]
    user_version = conn.execute("PRAGMA user_version").fetchone()[0]
    schema = conn.execute(
        "SELECT type, name, sql FROM sqlite_master "
        "WHERE name NOT LIKE 'sqlite_%' ORDER BY type, name"
    ).fetchall()
    table_names = [row[1] for row in schema if row[0] == "table"]
    if conn.execute("PRAGMA query_only").fetchone()[0] != 1:
        raise SystemExit("SQLite readback connection is not query_only")
    try:
        conn.execute("CREATE TABLE bb_read_only_probe (id INTEGER)")
    except sqlite3.OperationalError as error:
        if "readonly" not in str(error).lower() and "query_only" not in str(error).lower():
            raise SystemExit(f"SQLite write falsifier failed with an unexpected error: {error}")
    else:
        raise SystemExit("SQLite write falsifier unexpectedly succeeded")
finally:
    conn.close()

schema_hash = hashlib.sha256(json.dumps(schema, sort_keys=True).encode()).hexdigest()
doc = {
    "path": str(path),
    "journal_mode": str(journal_mode),
    "integrity": str(integrity),
    "user_version": user_version,
    "schema_hash": schema_hash,
    "write_falsified": True,
}
out.write_text(json.dumps(doc, sort_keys=True))
print(json.dumps(doc, sort_keys=True))
PY
}

check_primary_readback() {
  python3 - "$1" "$2" "$3" "$4" "$5" "$6" <<'PY'
import json
import pathlib
import sys

status_path, runs_path, dlq_path, config_path, before_path, after_path = sys.argv[1:]
status = json.loads(pathlib.Path(status_path).read_text())
runs = json.loads(pathlib.Path(runs_path).read_text())
dlq = json.loads(pathlib.Path(dlq_path).read_text())
config = json.loads(pathlib.Path(config_path).read_text())
before = json.loads(pathlib.Path(before_path).read_text())
after = json.loads(pathlib.Path(after_path).read_text())
if not isinstance(runs, list):
    raise SystemExit("GET /api/runs did not return a JSON list")
if not isinstance(dlq, list) or not all(isinstance(row, dict) for row in dlq):
    raise SystemExit("GET /api/dlq did not return a JSON object list")
backup = status.get("backup")
if not isinstance(backup, dict) or backup.get("enabled") is not True:
    raise SystemExit(f"local-primary backup is not enabled: {backup!r}")
if backup.get("status") != "fresh" or backup.get("healthy") is not True:
    raise SystemExit(f"local-primary backup is not fresh/healthy: {backup!r}")
age = backup.get("last_success_age_seconds")
rpo = backup.get("rpo_seconds")
if not isinstance(age, int) or not isinstance(rpo, int) or age > rpo:
    raise SystemExit(f"local-primary backup age exceeds RPO: age={age!r} rpo={rpo!r}")
heartbeat = pathlib.Path(config["heartbeat"])
if not heartbeat.exists():
    raise SystemExit(f"backup heartbeat missing: {heartbeat}")
open_rows = [row for row in dlq if row.get("status") == "open"]
summary_open_dlq = status.get("summary", {}).get("open_dlq", 0)
if not isinstance(summary_open_dlq, int) or summary_open_dlq < 0:
    raise SystemExit("status summary.open_dlq is not a non-negative integer")
if after.get("journal_mode", "").lower() != "wal":
    raise SystemExit(f"SQLite journal mode is not WAL: {after.get('journal_mode')!r}")
if after.get("integrity") != "ok":
    raise SystemExit(f"SQLite integrity_check failed: {after.get('integrity')!r}")
if after.get("write_falsified") is not True:
    raise SystemExit("SQLite readback did not prove its write falsifier")
for key in ("journal_mode", "integrity", "schema_hash", "user_version"):
    if before.get(key) != after.get(key):
        raise SystemExit(f"SQLite invariant changed during readback: {key}")
print(json.dumps({
    "bind": config["bind"],
    "runs": len(runs),
    "open_dlq": len(open_rows),
    "replayed_or_acknowledged_dlq": sum(row.get("status") in {"replayed", "acknowledged"} for row in dlq),
    "backup": {"status": backup["status"], "healthy": backup["healthy"], "age_seconds": age, "rpo_seconds": rpo},
    "sqlite": {"journal_mode": after["journal_mode"], "integrity": after["integrity"], "write_falsified": after["write_falsified"]},
}, sort_keys=True))
if open_rows or summary_open_dlq:
    print("READINESS BLOCKED: status=open dead letters must be resolved or explicitly acknowledged before enabling PR/merge loops", file=sys.stderr)
    if open_rows:
        print("open DLQ ids: " + ",".join(str(row.get("id")) for row in open_rows), file=sys.stderr)
    raise SystemExit(3)
PY
}

verify_launchd_primary() {
  if ! command -v launchctl >/dev/null 2>&1; then
    echo "FAIL launchd: launchctl is required for local-primary readback" >&2
    exit 1
  fi
  launchctl print "gui/$(id -u)/com.misty-step.bb-serve" >"$TMP/launchctl.txt" 2>&1 || {
    echo "FAIL launchd: label com.misty-step.bb-serve is not loaded" >&2
    cat "$TMP/launchctl.txt" >&2 || true
    exit 1
  }
  python3 - "$TMP/launchctl.txt" "$TMP/primary-config.json" <<'PY'
import json
import pathlib
import sys

text = pathlib.Path(sys.argv[1]).read_text()
config = json.loads(pathlib.Path(sys.argv[2]).read_text())
checkout = config["checkout"]
log_dir = pathlib.Path.home() / ".local" / "state" / "bitterblossom"
required = [
    "com.misty-step.bb-serve = {",
    f"program = {checkout}/scripts/bb-serve-local-entrypoint.sh",
    f"working directory = {checkout}",
    f"stdout path = {log_dir}/bb-serve.out.log",
    f"stderr path = {log_dir}/bb-serve.err.log",
]
missing = [needle for needle in required if needle not in text]
if missing:
    raise SystemExit("launchd ownership/readback mismatch: " + "; ".join(missing))
print("ok:launchd label=com.misty-step.bb-serve program=local-entrypoint workdir=" + checkout + " plane=" + config["root"])
PY
}

run_primary() {
  [ -f "$BB_CONFIG/plane.toml" ] || { echo "ops drill: configured live plane is missing $BB_CONFIG/plane.toml" >&2; exit 2; }
  TMP=$(mktemp -d /tmp/bb-ops-primary.XXXXXX)
  read_primary_config
  python3 - "$TMP/primary-config.json" <<'PY'
import json
import sys
c = json.load(open(sys.argv[1]))
expected = {"bind": "127.0.0.1:7093", "dev": False, "allow_local_substrate": True}
for key, value in expected.items():
    if c.get(key) != value:
        raise SystemExit(f"configured local-primary contract mismatch: {key}={c.get(key)!r}, expected {value!r}")
if not c.get("backup_enabled"):
    raise SystemExit("configured local-primary contract requires [backup].enabled = true")
if c.get("replica_env") != "LITESTREAM_REPLICA_URL":
    raise SystemExit("configured local-primary contract requires the named Litestream replica env")
PY
  bind=$(python3 -c 'import json,sys; print(json.load(open(sys.argv[1]))["bind"])' "$TMP/primary-config.json")
  db=$(python3 -c 'import json,sys; print(json.load(open(sys.argv[1]))["db"])' "$TMP/primary-config.json")
  sqlite_snapshot "$db" "$TMP/sqlite-before.json"
  verify_launchd_primary
  read_http_json primary-health "http://$bind/health"
  primary_api_auth "http://$bind/api/status"
  read_http_json primary-status "http://$bind/api/status"
  read_http_json primary-runs "http://$bind/api/runs"
  read_http_json primary-dlq "http://$bind/api/dlq"
  sqlite_snapshot "$db" "$TMP/sqlite-after.json"
  check_primary_readback "$TMP/primary-status.json" "$TMP/primary-runs.json" "$TMP/primary-dlq.json" "$TMP/primary-config.json" "$TMP/sqlite-before.json" "$TMP/sqlite-after.json"
  df -Pk "$db" | tail -n 1 | sed 's/^/resource_headroom: /'
  echo "ops drill: primary pass"
}

case "$MODE" in
  primary) run_primary ;;
  dev-temp) run_dev_temp ;;
esac
