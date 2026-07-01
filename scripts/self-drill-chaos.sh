#!/bin/sh
# Deterministic chaos drill for backlog 089. It builds an isolated dev plane,
# seeds a deliberately stale submission-storm arm, then proves `bb gate`
# settles through the durable notification outbox.
set -eu

cd "$(dirname "$0")/.."

REPORT=REPORT.json
TMP=

usage() {
  cat <<'USAGE'
usage: scripts/self-drill-chaos.sh [--report PATH]

Environment:
  BB_BIN   bb binary to use. Defaults to `cargo run --quiet --`.
USAGE
}

while [ "$#" -gt 0 ]; do
  case "$1" in
    --report)
      [ "$#" -ge 2 ] || { echo "self-drill: --report needs a path" >&2; exit 2; }
      REPORT=$2
      shift 2
      ;;
    -h | --help)
      usage
      exit 0
      ;;
    *)
      echo "self-drill: unknown argument: $1" >&2
      usage >&2
      exit 2
      ;;
  esac
done

cleanup() {
  if [ -n "$TMP" ]; then
    if [ "${BB_SELF_DRILL_KEEP_TMP:-0}" = "1" ]; then
      echo "kept self-drill temp plane: $TMP" >&2
    else
      rm -rf "$TMP"
    fi
  fi
}
trap cleanup EXIT INT TERM

need() {
  command -v "$1" >/dev/null 2>&1 || {
    echo "self-drill: missing required command: $1" >&2
    exit 2
  }
}

need python3

TMP=$(mktemp -d "${TMPDIR:-/tmp}/bb-self-drill.XXXXXX")
mkdir -p "$TMP/agents" "$TMP/tasks/correctness" "$TMP/tasks/security"

cat >"$TMP/plane.toml" <<'EOF'
dev = true

[notify]
webhook_url = "http://example.invalid/self-drill"

[gate]
required = ["correctness", "security"]
arm_timeout_seconds = 1
max_rounds = 3
EOF

cat >"$TMP/agents/stub.toml" <<'EOF'
version = 1
harness = "command"
model = ""
bin = "true"
EOF

for kind in correctness security; do
  cat >"$TMP/tasks/$kind/card.md" <<EOF
# Self-drill $kind

This task exists only inside the deterministic chaos drill.
EOF
  cat >"$TMP/tasks/$kind/task.toml" <<EOF
agent = "stub"
substrate = "local"
verdict = "$kind"

[[trigger]]
kind = "manual"
EOF
done

cat >"$TMP/notify-stub.sh" <<EOF
#!/bin/sh
cat >> "$TMP/notify.log"
EOF
chmod +x "$TMP/notify-stub.sh"

bb() {
  if [ -n "${BB_BIN:-}" ]; then
    "$BB_BIN" --config "$TMP" "$@"
  else
    cargo run --quiet -- --config "$TMP" "$@"
  fi
}

bb check >/dev/null
submission_json=$(bb submit open --change self-drill-chaos --rev drill-rev --json)
submission_id=$(printf '%s' "$submission_json" | python3 -c 'import json,sys; print(json.load(sys.stdin)["id"])')

python3 - "$TMP/.bb/plane.db" "$submission_id" <<'PY'
import datetime
import json
import sqlite3
import sys

db, sub = sys.argv[1], sys.argv[2]
now = datetime.datetime.now(datetime.timezone.utc)
fresh = now.isoformat().replace("+00:00", "Z")
stale = (now - datetime.timedelta(seconds=5)).isoformat().replace("+00:00", "Z")
conn = sqlite3.connect(db)
conn.execute(
    """INSERT INTO runs
       (id, task, trigger_kind, idempotency_key, state, trace_id, payload, created_at, updated_at)
       VALUES (?, 'correctness', 'manual', ?, 'success', ?, ?, ?, ?)""",
    (
        f"drill-correctness-{sub}",
        f"storm:{sub}:correctness",
        f"trace-correctness-{sub}",
        json.dumps({"submission": sub}),
        fresh,
        fresh,
    ),
)
conn.execute(
    """INSERT INTO verdicts
       (submission_id, run_id, kind, verdict, findings_json, created_at)
       VALUES (?, ?, 'correctness', 'pass', '[]', ?)""",
    (sub, f"drill-correctness-{sub}", fresh),
)
conn.execute(
    """INSERT INTO runs
       (id, task, trigger_kind, idempotency_key, state, trace_id, payload, created_at, updated_at)
       VALUES (?, 'security', 'manual', ?, 'running', ?, ?, ?, ?)""",
    (
        f"drill-security-{sub}",
        f"storm:{sub}:security",
        f"trace-security-{sub}",
        json.dumps({"submission": sub}),
        stale,
        stale,
    ),
)
conn.commit()
PY

gate_json=$(BB_NOTIFY_BIN="$TMP/notify-stub.sh" bb gate --submission "$submission_id" --json)
outbox_json=$(bb notify list --json)
notify_log=$(cat "$TMP/notify.log")

python3 - "$REPORT" "$submission_id" "$gate_json" "$outbox_json" "$notify_log" <<'PY'
import json
import pathlib
import sys

report_path, sub, gate_raw, outbox_raw, notify_log = sys.argv[1:]
gate = json.loads(gate_raw)
outbox = json.loads(outbox_raw)
security = next((m for m in gate["members"] if m["kind"] == "security"), None)
rows = [r for r in outbox if r["event"] == "submission_escalated"]
errors = []
if gate.get("decision") != "escalated":
    errors.append(f"gate decision {gate.get('decision')!r}, want escalated")
if not security or security.get("status") != "run:timed_out":
    errors.append(f"security member {security!r}, want run:timed_out")
if not rows:
    errors.append("submission_escalated outbox row missing")
else:
    row = rows[0]
    if row.get("status") != "delivered":
        errors.append(f"outbox status {row.get('status')!r}, want delivered")
    if row.get("attempts") != 1:
        errors.append(f"outbox attempts {row.get('attempts')!r}, want 1")
if "submission_escalated" not in notify_log or "timed_out_members" not in notify_log:
    errors.append("notify log missing submission_escalated timed_out_members payload")

doc = {
    "status": "fail" if errors else "pass",
    "artifact_paths": ["REPORT.json"],
    "drill": "submission_arm_timeout_outbox_notification",
    "submission": sub,
    "gate_decision": gate.get("decision"),
    "security_status": None if security is None else security.get("status"),
    "outbox_event": None if not rows else rows[0].get("event"),
    "outbox_status": None if not rows else rows[0].get("status"),
    "outbox_attempts": None if not rows else rows[0].get("attempts"),
    "notification_observed": "submission_escalated" in notify_log,
    "errors": errors,
}
pathlib.Path(report_path).write_text(json.dumps(doc, indent=2) + "\n", encoding="utf-8")
if errors:
    for error in errors:
        print(f"self-drill: {error}", file=sys.stderr)
    sys.exit(1)
print(json.dumps({"schema_version": "bb.command_result.v1", "result": "self-drill pass"}))
PY
