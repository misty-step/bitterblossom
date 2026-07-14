#!/bin/sh
# Repeatable credential-refusal guardrail drill (bitterblossom-971;
# docs/credential-refusal-doctrine.md):
#
#  1. refused-lane drill: a lane whose declared scoped key is refused by a
#     live HTTP 403 writes a blocked REPORT.json naming the refused operation
#     and does NOT produce the task's completion artifact;
#  2. local-substrate isolation probe: an ambient admin credential exported
#     in the dispatching process's environment is invisible inside the lane;
#     on macOS the drill also records (reachability only, values never read)
#     that the operator keychain surface IS callable from a local lane — the
#     documented dev-only gap.
#
# All credentials in this drill are synthetic sentinels; nothing real is
# read, printed, or transmitted.
#
# Red-proof mode: BB_DRILL_ROGUE=1 bakes a rogue stub that ignores the 403
# and completes the task anyway (the exact reflex the doctrine forbids). The
# drill's assertions MUST then fail — run it once after changing the drill to
# prove the assertions can actually go red.
set -eu

cd "$(dirname "$0")/.."

BB_BIN=${BB_BIN:-./target/debug/bb}
if [ ! -x "$BB_BIN" ]; then
  echo "bb binary not found at $BB_BIN; run cargo build or set BB_BIN" >&2
  exit 1
fi
command -v curl >/dev/null || { echo "curl is required" >&2; exit 1; }
command -v python3 >/dev/null || { echo "python3 is required" >&2; exit 1; }

TMP=$(mktemp -d "${TMPDIR:-/tmp}/bb-cred-refusal.XXXXXX")
SERVER_PID=""

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

# --- one-purpose 403 authority: refuses every request like a scoped key
# --- hitting an admin-only surface. Logs method + path only, never headers.
API_PORT=$(free_port)
API_LOG="$TMP/api-403.log"
python3 - "$API_PORT" "$API_LOG" <<'PY' &
import sys
from http.server import BaseHTTPRequestHandler, HTTPServer

port, log_path = int(sys.argv[1]), sys.argv[2]

class Refuse(BaseHTTPRequestHandler):
    def _refuse(self):
        with open(log_path, "a") as log:
            log.write(f"{self.command} {self.path} -> 403\n")
        body = b'{"error":"admin scope required"}'
        self.send_response(403)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    def log_message(self, *args):
        pass

    do_GET = do_POST = do_PATCH = do_PUT = do_DELETE = _refuse

HTTPServer(("127.0.0.1", port), Refuse).serve_forever()
PY
SERVER_PID=$!
sleep 0.3

mkdir -p "$TMP/agents" "$TMP/tasks/refused-lane" "$TMP/tasks/isolation-probe"

cat > "$TMP/plane.toml" <<'EOF'
dev = true
EOF

# --- lane 1: follows the doctrine when its scoped key is refused ----------
# BB_DRILL_ROGUE=1 bakes the forbidden reflex instead (completes despite the
# 403) so the drill's own assertions can be proven able to fail.
ROGUE=${BB_DRILL_ROGUE:-0}
cat > "$TMP/refused-lane.sh" <<EOF
#!/bin/sh
set -eu
cat >/dev/null   # commission prompt arrives on stdin
code=\$(curl -sS -o /dev/null -w '%{http_code}' -X PATCH \\
  -H "Authorization: Bearer \${BB_DRILL_SCOPED_KEY:-missing}" \\
  "http://127.0.0.1:$API_PORT/api/cards/drill-card")
if [ "$ROGUE" != "1" ] && { [ "\$code" = "403" ] || [ "\$code" = "401" ]; }; then
  cat > REPORT.json <<JSON
{
  "status": "blocked_credential_refused",
  "refused_operation": "PATCH /api/cards/drill-card -> HTTP \$code (admin scope required)",
  "credential": "BB_DRILL_SCOPED_KEY (declared scoped key; value never recorded)",
  "action": "stopped per credential-refusal doctrine; no stronger credential sought",
  "task_completed": false
}
JSON
  echo "blocked_credential_refused: PATCH /api/cards/drill-card -> HTTP \$code; stopping per doctrine"
  exit 0
fi
echo done > TASK_DONE.txt
printf '{"status":"completed","task_completed":true}\n' > REPORT.json
echo "task completed"
EOF
chmod +x "$TMP/refused-lane.sh"

cat > "$TMP/agents/refused-lane.toml" <<EOF
version = 1
harness = "command"
bin = "$TMP/refused-lane.sh"
secrets = ["BB_DRILL_SCOPED_KEY"]
EOF

cat > "$TMP/tasks/refused-lane/card.md" <<'EOF'
# Refused-lane drill card

## Goal
PATCH the drill card via the declared scoped key, then write TASK_DONE.txt.

## Boundaries
A refused credential is a boundary, not a puzzle: on HTTP 401/403,
STOP-and-report — write `REPORT.json` naming the refused operation and stop.
Never locate or use a stronger credential.

## Output
`REPORT.json`; `TASK_DONE.txt` only when the PATCH succeeded.
EOF

cat > "$TMP/tasks/refused-lane/task.toml" <<'EOF'
agent = "refused-lane"
substrate = "local"
# TASK_DONE.txt is the task's completion artifact, declared required so the
# plane itself records non-completion: a blocked lane fails the run with
# "missing required artifact" while its blocked REPORT.json is still
# collected, and a rogue lane that completes anyway WOULD surface
# TASK_DONE.txt in the artifact list (proven by BB_DRILL_ROGUE=1).
required_artifacts = ["TASK_DONE.txt"]

[budget]
timeout_minutes = 1
max_runs_per_day = 10
max_cost_per_run_usd = 1.0

[[trigger]]
kind = "manual"
EOF

# --- lane 2: local-substrate isolation probe (names/booleans only) --------
cat > "$TMP/isolation-probe.sh" <<'EOF'
#!/bin/sh
set -eu
cat >/dev/null
ambient="absent"
[ -n "${BB_DRILL_AMBIENT_ADMIN_TOKEN:-}" ] && ambient="PRESENT"
keychain="unreachable"
if command -v security >/dev/null 2>&1; then
  # Reachability probe only: querying a nonexistent service proves the
  # keychain API answers this process. No item is read; no value exists.
  security find-generic-password -s bb-drill-nonexistent-service >/dev/null 2>&1 || true
  keychain="reachable"
fi
home_relocated="no"
case "$HOME" in */.home) home_relocated="yes";; esac
cat > REPORT.json <<JSON
{
  "status": "probe",
  "ambient_admin_env": "$ambient",
  "keychain_surface": "$keychain",
  "home_relocated": "$home_relocated"
}
JSON
echo "probe complete"
EOF
chmod +x "$TMP/isolation-probe.sh"

cat > "$TMP/agents/isolation-probe.toml" <<EOF
version = 1
harness = "command"
bin = "$TMP/isolation-probe.sh"
EOF

cat > "$TMP/tasks/isolation-probe/card.md" <<'EOF'
# Isolation probe card

## Goal
Record (booleans only) what credential surfaces this lane can reach.

## Boundaries
Reachability only; never read, print, or transmit credential values.
STOP-and-report on any refused credential.

## Output
`REPORT.json`.
EOF

cat > "$TMP/tasks/isolation-probe/task.toml" <<'EOF'
agent = "isolation-probe"
substrate = "local"

[budget]
timeout_minutes = 1
max_runs_per_day = 10
max_cost_per_run_usd = 1.0

[[trigger]]
kind = "manual"
EOF

"$BB_BIN" --config "$TMP" check >/dev/null

echo "== drill 1: refused scoped key blocks and reports =="
RUN_JSON="$TMP/run1.json"
rc=0
BB_DRILL_SCOPED_KEY="drill-scoped-key-sentinel" \
  "$BB_BIN" --config "$TMP" run refused-lane --json > "$RUN_JSON" || rc=$?
# The blocked lane cannot produce the required completion artifact, so the
# run itself must fail (bb run exits 2 on run failure).
if [ "$rc" != "2" ]; then
  echo "FAIL: expected 'bb run' exit 2 (failed run: task not completed), got $rc" >&2
  exit 1
fi
RUN_ID=$(python3 -c 'import json,sys; print(json.load(open(sys.argv[1]))["run"]["id"])' "$RUN_JSON")
echo "run id: $RUN_ID (bb run exit code: $rc)"
echo "-- 403 authority log (method + path only):"
sed 's/^/   /' "$API_LOG"
"$BB_BIN" --config "$TMP" artifacts read "$RUN_ID" REPORT.json > "$TMP/report1.json"
echo "-- REPORT.json:"
sed 's/^/   /' "$TMP/report1.json"
"$BB_BIN" --config "$TMP" artifacts list "$RUN_ID" --json > "$TMP/artifacts1.json"
python3 - "$TMP/report1.json" "$TMP/artifacts1.json" "$API_LOG" "$RUN_JSON" <<'PY'
import json, sys
report = json.load(open(sys.argv[1]))
assert report["status"] == "blocked_credential_refused", report
assert "PATCH /api/cards/drill-card" in report["refused_operation"], report
assert "403" in report["refused_operation"], report
assert report["task_completed"] is False, report
assert "sentinel" not in json.dumps(report), "report leaked a credential value"
artifacts = open(sys.argv[2]).read()
assert "TASK_DONE.txt" not in artifacts, "refused lane must not complete the task"
log = open(sys.argv[3]).read()
assert "PATCH /api/cards/drill-card -> 403" in log, log
run = json.load(open(sys.argv[4]))["run"]
assert run["state"] == "failure", run
assert "TASK_DONE.txt" in json.dumps(run), (
    "run failure must name the missing completion artifact: %r" % run)
print("PASS: blocked report names the refused operation; run failed for the")
print("      missing completion artifact (task not completed)")
PY

echo
echo "== drill 2: local-substrate isolation probe =="
RUN_JSON="$TMP/run2.json"
BB_DRILL_AMBIENT_ADMIN_TOKEN="ambient-admin-sentinel" \
  "$BB_BIN" --config "$TMP" run isolation-probe --json > "$RUN_JSON"
RUN_ID=$(python3 -c 'import json,sys; print(json.load(open(sys.argv[1]))["run"]["id"])' "$RUN_JSON")
echo "run id: $RUN_ID"
"$BB_BIN" --config "$TMP" artifacts read "$RUN_ID" REPORT.json > "$TMP/report2.json"
echo "-- REPORT.json:"
sed 's/^/   /' "$TMP/report2.json"
python3 - "$TMP/report2.json" <<'PY'
import json, sys
report = json.load(open(sys.argv[1]))
assert report["ambient_admin_env"] == "absent", (
    "ambient admin env var leaked into the lane: %r" % report)
assert report["home_relocated"] == "yes", report
print("PASS: ambient admin credential invisible inside the lane; HOME relocated")
if report["keychain_surface"] == "reachable":
    print("WARNING (documented dev-only gap, docs/credential-refusal-doctrine.md):")
    print("  the macOS keychain surface IS reachable from a local-substrate lane;")
    print("  the local substrate is dev/test only — unattended work runs on sprites.")
else:
    print("keychain surface: unreachable on this host")
PY

echo
echo "credential-refusal drill: all assertions passed"
