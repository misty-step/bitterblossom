#!/bin/sh
# Repeatable live drill for the bb control loop:
# - loopback read API and HTML with no token;
# - bearer-only read API and HTML with BB_API_TOKEN;
# - signed webhook burst against a dev plane with max_runs_per_day containment.
set -eu

cd "$(dirname "$0")/.."

BB_BIN=${BB_BIN:-./target/debug/bb}
if [ ! -x "$BB_BIN" ]; then
  echo "bb binary not found at $BB_BIN; run cargo build or set BB_BIN" >&2
  exit 1
fi
command -v curl >/dev/null || { echo "curl is required" >&2; exit 1; }
command -v python3 >/dev/null || { echo "python3 is required" >&2; exit 1; }

TMP=$(mktemp -d "${TMPDIR:-/tmp}/bb-control-loop.XXXXXX")
SERVER_PID=""
SERVER_LOG=""
PORT=""
SECRET="drill-secret"
TOKEN="drill-token"
NOTIFY_LOG="$TMP/notify.log"

cleanup() {
  if [ -n "$SERVER_PID" ]; then
    kill "$SERVER_PID" 2>/dev/null || true
    wait "$SERVER_PID" 2>/dev/null || true
  fi
  if [ "${BB_DRILL_KEEP_TMP:-0}" = "1" ]; then
    echo "kept temp plane: $TMP"
  else
    rm -rf "$TMP"
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

write_drill_plane() {
  mkdir -p "$TMP/agents" "$TMP/tasks/demo"
  cat > "$TMP/plane.toml" <<'EOF'
dev = true

[ingress]
bind = "127.0.0.1:0"

[notify]
webhook_url = "http://example.invalid/hook"
EOF
  cat > "$TMP/stub-command.sh" <<'EOF'
#!/bin/sh
cat >/dev/null
sleep 0.1
echo ok
EOF
  chmod +x "$TMP/stub-command.sh"
  cat > "$TMP/notify-stub.sh" <<'EOF'
#!/bin/sh
cat >> "$BB_NOTIFY_LOG"
echo >> "$BB_NOTIFY_LOG"
EOF
  chmod +x "$TMP/notify-stub.sh"
  cat > "$TMP/agents/stub.toml" <<EOF
version = 1
harness = "command"
bin = "$TMP/stub-command.sh"
EOF
  cat > "$TMP/tasks/demo/card.md" <<'EOF'
# Demo drill

Return success.
EOF
  cat > "$TMP/tasks/demo/task.toml" <<'EOF'
agent = "stub"
substrate = "local"

[budget]
timeout_minutes = 1
max_runs_per_day = 1
max_cost_per_run_usd = 1.0

[[trigger]]
kind = "manual"

[[trigger]]
kind = "webhook"
route = "demo"
secret_env = "BB_DRILL_HOOK_SECRET"
dedupe_key = "header:X-GitHub-Delivery"
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
    if [ "$code" = "200" ]; then
      return
    fi
    i=$((i + 1))
    sleep 0.1
  done
  echo "bb serve did not become healthy on port $PORT; log follows" >&2
  cat "$SERVER_LOG" >&2 || true
  exit 1
}

start_server() {
  label=$1
  token=${2:-}
  PORT=$(free_port)
  SERVER_LOG="$TMP/serve-$label.log"
  if [ -n "$token" ]; then
    BB_INGRESS_BIND="127.0.0.1:$PORT" \
      BB_DRILL_HOOK_SECRET="$SECRET" \
      BB_NOTIFY_BIN="$TMP/notify-stub.sh" \
      BB_NOTIFY_LOG="$NOTIFY_LOG" \
      BB_API_TOKEN="$token" \
      "$BB_BIN" --config "$TMP" serve >"$SERVER_LOG" 2>&1 &
  else
    env -u BB_API_TOKEN \
      BB_INGRESS_BIND="127.0.0.1:$PORT" \
      BB_DRILL_HOOK_SECRET="$SECRET" \
      BB_NOTIFY_BIN="$TMP/notify-stub.sh" \
      BB_NOTIFY_LOG="$NOTIFY_LOG" \
      "$BB_BIN" --config "$TMP" serve >"$SERVER_LOG" 2>&1 &
  fi
  SERVER_PID=$!
  wait_for_server
  echo "serve:$label port=$PORT"
}

stop_server() {
  if [ -n "$SERVER_PID" ]; then
    kill "$SERVER_PID" 2>/dev/null || true
    wait "$SERVER_PID" 2>/dev/null || true
    SERVER_PID=""
  fi
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
    echo "serve log:" >&2
    cat "$SERVER_LOG" >&2 || true
    exit 1
  fi
  echo "ok:$label http=$code"
}

signature_for() {
  BODY=$1 SECRET=$SECRET python3 - <<'PY'
import hashlib, hmac, os
print("sha256=" + hmac.new(
    os.environ["SECRET"].encode(),
    os.environ["BODY"].encode(),
    hashlib.sha256,
).hexdigest())
PY
}

post_delivery() {
  n=$1
  body="{\"delivery\":$n}"
  sig=$(signature_for "$body")
  expect_code "webhook-$n" 202 \
    -H "X-Hub-Signature-256: $sig" \
    -H "X-GitHub-Delivery: drill-$n" \
    -H "Content-Type: application/json" \
    -d "$body" \
    "http://127.0.0.1:$PORT/hooks/demo"
}

wait_for_storm() {
  i=0
  while [ "$i" -lt 120 ]; do
    "$BB_BIN" --config "$TMP" runs list --json > "$TMP/runs.json"
    if python3 - "$TMP/runs.json" <<'PY'
import json, sys
runs = json.load(open(sys.argv[1]))
active = [r for r in runs if r["state"] in ("pending", "running")]
sys.exit(0 if len(runs) >= 5 and not active else 1)
PY
    then
      return
    fi
    i=$((i + 1))
    sleep 0.1
  done
  echo "storm did not settle; runs:" >&2
  cat "$TMP/runs.json" >&2 || true
  exit 1
}

assert_storm() {
  "$BB_BIN" --config "$TMP" runs list --json > "$TMP/runs.json"
  "$BB_BIN" --config "$TMP" task list --json > "$TMP/tasks.json"
  python3 - "$TMP/runs.json" "$TMP/tasks.json" "$NOTIFY_LOG" <<'PY'
import json, pathlib, sys
runs = sorted(json.load(open(sys.argv[1])), key=lambda r: r["created_at"])
states = [r["state"] for r in runs]
if states != ["success", "blocked_budget", "blocked_budget", "blocked_budget", "blocked_budget"]:
    raise SystemExit(f"unexpected run states: {states}")
tasks = json.load(open(sys.argv[2]))
demo = next(t for t in tasks if t["task"] == "demo")
parked = demo.get("parked") or ""
if "max_runs_per_day" not in parked:
    raise SystemExit(f"demo task not parked by max_runs_per_day: {parked!r}")
notify = pathlib.Path(sys.argv[3]).read_text() if pathlib.Path(sys.argv[3]).exists() else ""
if "budget_blocked" not in notify:
    raise SystemExit("budget_blocked notification was not recorded")
print(f"storm: total={len(runs)} states={','.join(states)} parked={parked}")
print(f"storm: notifications={notify.count('budget_blocked')}")
PY
}

write_drill_plane
"$BB_BIN" --config "$TMP" check >/dev/null

start_server open
expect_code "open-health" 200 "http://127.0.0.1:$PORT/health"
expect_code "open-status" 200 "http://127.0.0.1:$PORT/api/status"
expect_code "open-tasks" 200 "http://127.0.0.1:$PORT/api/tasks"
expect_code "open-runs" 200 "http://127.0.0.1:$PORT/api/runs"
expect_code "open-dlq" 200 "http://127.0.0.1:$PORT/api/dlq"
expect_code "open-notify" 200 "http://127.0.0.1:$PORT/api/notify"
expect_code "open-leases" 200 "http://127.0.0.1:$PORT/api/leases"
expect_code "open-ingress" 200 "http://127.0.0.1:$PORT/api/ingress"
expect_code "open-export" 200 "http://127.0.0.1:$PORT/api/export"
expect_code "open-submissions" 200 "http://127.0.0.1:$PORT/api/submissions"
expect_code "open-html" 200 "http://127.0.0.1:$PORT/"

for n in 1 2 3 4 5; do
  post_delivery "$n"
done
wait_for_storm
assert_storm
stop_server

start_server token "$TOKEN"
expect_code "token-health-noauth" 200 "http://127.0.0.1:$PORT/health"
expect_code "token-status-noauth" 401 "http://127.0.0.1:$PORT/api/status"
expect_code "token-status-query" 401 "http://127.0.0.1:$PORT/api/status?token=$TOKEN"
expect_code "token-status-bad" 401 -H "Authorization: Bearer wrong" "http://127.0.0.1:$PORT/api/status"
expect_code "token-status-bearer" 200 -H "Authorization: Bearer $TOKEN" "http://127.0.0.1:$PORT/api/status"
expect_code "token-tasks-bearer" 200 -H "Authorization: Bearer $TOKEN" "http://127.0.0.1:$PORT/api/tasks"
expect_code "token-runs-bearer" 200 -H "Authorization: Bearer $TOKEN" "http://127.0.0.1:$PORT/api/runs"
expect_code "token-dlq-bearer" 200 -H "Authorization: Bearer $TOKEN" "http://127.0.0.1:$PORT/api/dlq"
expect_code "token-notify-bearer" 200 -H "Authorization: Bearer $TOKEN" "http://127.0.0.1:$PORT/api/notify"
expect_code "token-leases-bearer" 200 -H "Authorization: Bearer $TOKEN" "http://127.0.0.1:$PORT/api/leases"
expect_code "token-ingress-bearer" 200 -H "Authorization: Bearer $TOKEN" "http://127.0.0.1:$PORT/api/ingress"
expect_code "token-export-bearer" 200 -H "Authorization: Bearer $TOKEN" "http://127.0.0.1:$PORT/api/export"
expect_code "token-submissions-bearer" 200 -H "Authorization: Bearer $TOKEN" "http://127.0.0.1:$PORT/api/submissions"
expect_code "token-html-bearer" 200 -H "Authorization: Bearer $TOKEN" "http://127.0.0.1:$PORT/"
stop_server

echo "control-loop drill: pass"
