#!/bin/sh
# Thin command-harness wrapper for the incident triage responder. It validates
# runtime inputs, invokes the GLM responder, enforces the fix-attempt guard in
# REPORT.json, and emits bb.command_result.v1 telemetry.
set -eu

event_file="${BB_EVENT_FILE:-EVENT.json}"
run_file="${BB_RUN_FILE:-RUN.json}"
model="${INCIDENT_TRIAGE_MODEL:-z-ai/glm-5.2}"
provider="${INCIDENT_TRIAGE_PROVIDER:-openrouter}"
agent_bin="${INCIDENT_TRIAGE_AGENT_BIN:-pi}"
agent_npx_package="${INCIDENT_TRIAGE_AGENT_NPX_PACKAGE:-@earendil-works/pi-coding-agent@0.80.3}"
agent_thinking="${INCIDENT_TRIAGE_THINKING:-off}"
agent_stdout_max_bytes="${INCIDENT_TRIAGE_AGENT_STDOUT_MAX_BYTES:-10485760}"
max_fix_attempts="${INCIDENT_TRIAGE_MAX_FIX_ATTEMPTS:-3}"
agent_out_dir="${INCIDENT_TRIAGE_OUT_DIR:-incident-triage}"
prompt_file="INCIDENT_TRIAGE_PROMPT.md"

cat > .bb-wrapper-stdin.txt || true

validate_env_name() {
  name="$1"
  value="$2"
  stripped=$(printf '%s' "$value" | tr -d 'A-Za-z0-9_')
  case "$value" in
    [A-Za-z_]*) ;;
    *)
      echo "$name must be a valid environment variable name, got: $value" >&2
      exit 64
      ;;
  esac
  if [ -n "$stripped" ]; then
    echo "$name must be a valid environment variable name, got: $value" >&2
    exit 64
  fi
}

copy_env_value() {
  target="$1"
  source="$2"
  validate_env_name "${target}_ENV" "$source"
  value="$(eval "printf '%s' \"\${${source}:-}\"")"
  if [ -n "$value" ]; then
    case "$target" in
      GH_TOKEN) GH_TOKEN="$value"; export GH_TOKEN ;;
      OPENROUTER_API_KEY) OPENROUTER_API_KEY="$value"; export OPENROUTER_API_KEY ;;
      CANARY_API_KEY) CANARY_API_KEY="$value"; export CANARY_API_KEY ;;
      *)
        echo "internal error: unsupported target env $target" >&2
        exit 70
        ;;
    esac
  fi
}

copy_env_value GH_TOKEN "${INCIDENT_TRIAGE_GH_TOKEN_ENV:-GH_TOKEN}"
copy_env_value OPENROUTER_API_KEY "${INCIDENT_TRIAGE_OPENROUTER_API_KEY_ENV:-OPENROUTER_API_KEY}"
copy_env_value CANARY_API_KEY "${INCIDENT_TRIAGE_CANARY_API_KEY_ENV:-CANARY_API_KEY}"

if [ ! -f "$event_file" ]; then
  echo "EVENT.json not found" >&2
  exit 64
fi
if [ ! -f "$run_file" ]; then
  echo "RUN.json not found" >&2
  exit 64
fi
case "$max_fix_attempts" in
  ''|*[!0-9]*)
    echo "INCIDENT_TRIAGE_MAX_FIX_ATTEMPTS must be a positive integer" >&2
    exit 64
    ;;
esac
if [ "$max_fix_attempts" -lt 1 ]; then
  echo "INCIDENT_TRIAGE_MAX_FIX_ATTEMPTS must be a positive integer" >&2
  exit 64
fi
case "$agent_stdout_max_bytes" in
  ''|*[!0-9]*)
    echo "INCIDENT_TRIAGE_AGENT_STDOUT_MAX_BYTES must be a positive integer" >&2
    exit 64
    ;;
esac
if [ "$agent_stdout_max_bytes" -lt 4096 ]; then
  echo "INCIDENT_TRIAGE_AGENT_STDOUT_MAX_BYTES must be at least 4096" >&2
  exit 64
fi

eval "$(
  python3 - "$event_file" "$run_file" <<'PY'
import json
import shlex
import sys

event_path, run_path = sys.argv[1], sys.argv[2]
with open(event_path, "r", encoding="utf-8") as f:
    event = json.load(f)
with open(run_path, "r", encoding="utf-8") as f:
    run = json.load(f)

incident = event.get("incident") if isinstance(event.get("incident"), dict) else {}
subject = event.get("subject") if isinstance(event.get("subject"), dict) else {}
signal = event.get("signal") if isinstance(event.get("signal"), dict) else {}
incident_id = incident.get("id") or subject.get("id") or ""
service = incident.get("service") or subject.get("service") or ""
severity = incident.get("severity") or signal.get("severity") or ""
fingerprint = signal.get("fingerprint") or ""
run_id = run.get("run_id") or run.get("id") or ""
delivery_id = (run.get("trigger") or {}).get("idempotency_key") or ""
if delivery_id.startswith("wh:incident-triage:"):
    delivery_id = delivery_id[len("wh:incident-triage:"):]
elif delivery_id.startswith("wh:canary-triage:"):
    delivery_id = delivery_id[len("wh:canary-triage:"):]

repos = {
    "canary": ("misty-step/canary", "canary", "true"),
    "bastion": ("misty-step/bastion", "bastion", "false"),
    "powder": ("misty-step/powder", "powder", "false"),
}
repo, repo_dir, auto_deploy = repos.get(service, ("", "", "false"))
allowed = "1" if service in repos else "0"

for key, value in {
    "BB_TRIAGE_RUN_ID": run_id,
    "BB_TRIAGE_DELIVERY_ID": delivery_id,
    "BB_TRIAGE_INCIDENT_ID": incident_id,
    "BB_TRIAGE_SERVICE": service,
    "BB_TRIAGE_SEVERITY": severity,
    "BB_TRIAGE_FINGERPRINT": fingerprint,
    "BB_TRIAGE_REPO": repo,
    "BB_TRIAGE_REPO_DIR": repo_dir,
    "BB_TRIAGE_AUTO_DEPLOY_ON_MERGE": auto_deploy,
    "BB_TRIAGE_ALLOWED": allowed,
}.items():
    print(f"{key}={shlex.quote(str(value))}")
PY
)"
export BB_TRIAGE_RUN_ID BB_TRIAGE_DELIVERY_ID BB_TRIAGE_INCIDENT_ID
export BB_TRIAGE_SERVICE BB_TRIAGE_SEVERITY BB_TRIAGE_FINGERPRINT
export BB_TRIAGE_REPO BB_TRIAGE_REPO_DIR BB_TRIAGE_AUTO_DEPLOY_ON_MERGE BB_TRIAGE_ALLOWED

write_blocked_report() {
  reason="$1"
  python3 - "$reason" "$max_fix_attempts" <<'PY'
import json
import os
import sys

reason, max_fix_attempts = sys.argv[1], int(sys.argv[2])
report = {
    "schema": "bb.incident_triage_response.v1",
    "status": "blocked",
    "bb_run_id": os.environ.get("BB_TRIAGE_RUN_ID", ""),
    "delivery_id": os.environ.get("BB_TRIAGE_DELIVERY_ID", ""),
    "incident": {
        "id": os.environ.get("BB_TRIAGE_INCIDENT_ID", ""),
        "service": os.environ.get("BB_TRIAGE_SERVICE", ""),
        "severity": os.environ.get("BB_TRIAGE_SEVERITY", ""),
        "fingerprint": os.environ.get("BB_TRIAGE_FINGERPRINT", ""),
    },
    "repo": os.environ.get("BB_TRIAGE_REPO", ""),
    "progress_writebacks": [],
    "hypotheses": [],
    "experiments": [],
    "fix_attempts": [],
    "iteration_guard": {
        "max_fix_attempts": max_fix_attempts,
        "attempts_used": 0,
        "stopped": True,
        "reason": reason,
    },
    "scope_honesty": {
        "auto_deploy_on_merge": os.environ.get("BB_TRIAGE_AUTO_DEPLOY_ON_MERGE") == "true",
        "v1_stop": "blocked_before_agent",
    },
    "artifact_paths": ["REPORT.json"],
    "residual_risk": [reason],
}
with open("REPORT.json", "w", encoding="utf-8") as f:
    json.dump(report, f, indent=2, sort_keys=True)
    f.write("\n")
print(json.dumps({
    "schema_version": "bb.command_result.v1",
    "result": f"incident triage blocked: {reason}",
}, sort_keys=True))
PY
}

write_report_from_terminal_writebacks() {
  reason="$1"
  python3 - "$reason" "$max_fix_attempts" "$agent_out_dir" <<'PY'
import glob
import json
import os
import sys

reason, max_fix_attempts_raw, out_dir = sys.argv[1:4]
max_fix_attempts = int(max_fix_attempts_raw)
receipt_paths = []
for pattern in [
    os.path.join(out_dir, "writebacks", "*.json"),
    os.path.join(out_dir, "*writeback*.json"),
]:
    receipt_paths.extend(glob.glob(pattern))
receipt_paths = sorted(set(receipt_paths))

progress = []
terminal_seen = False
terminal_actions = {
    "closed",
    "dismissed",
    "no-defect",
    "no-fix-needed",
    "resolved",
    "terminal",
}
for path in receipt_paths:
    try:
        with open(path, "r", encoding="utf-8") as f:
            receipt = json.load(f)
    except (OSError, ValueError):
        continue
    if not isinstance(receipt, dict):
        continue
    action = (
        receipt.get("action")
        or receipt.get("kind")
        or receipt.get("event")
        or "canary-writeback"
    )
    ref = (
        receipt.get("ref")
        or receipt.get("annotation_id")
        or receipt.get("id")
        or receipt.get("url")
        or os.path.relpath(path)
    )
    normalized_action = str(action).lower().replace("_", "-")
    terminal = bool(receipt.get("terminal")) or normalized_action in terminal_actions
    terminal_seen = terminal_seen or terminal
    progress.append({
        "action": action,
        "ref": ref,
        "receipt_path": os.path.relpath(path),
        "terminal": terminal,
    })

if not progress or not terminal_seen:
    raise SystemExit(2)

report = {
    "schema": "bb.incident_triage_response.v1",
    "status": "canary_writebacks_preserved",
    "bb_run_id": os.environ.get("BB_TRIAGE_RUN_ID", ""),
    "delivery_id": os.environ.get("BB_TRIAGE_DELIVERY_ID", ""),
    "incident": {
        "id": os.environ.get("BB_TRIAGE_INCIDENT_ID", ""),
        "service": os.environ.get("BB_TRIAGE_SERVICE", ""),
        "severity": os.environ.get("BB_TRIAGE_SEVERITY", ""),
        "fingerprint": os.environ.get("BB_TRIAGE_FINGERPRINT", ""),
    },
    "repo": os.environ.get("BB_TRIAGE_REPO", ""),
    "progress_writebacks": progress,
    "hypotheses": [],
    "experiments": [],
    "fix_attempts": [],
    "iteration_guard": {
        "max_fix_attempts": max_fix_attempts,
        "attempts_used": 0,
        "stopped": True,
        "reason": reason,
    },
    "scope_honesty": {
        "auto_deploy_on_merge": os.environ.get("BB_TRIAGE_AUTO_DEPLOY_ON_MERGE") == "true",
        "v1_stop": "agent_failed_after_canary_terminal_writebacks",
    },
    "writeback_receipts": [os.path.relpath(path) for path in receipt_paths],
    "artifact_paths": ["REPORT.json"],
    "residual_risk": [
        f"{reason}; synthesized REPORT.json from Canary terminal writeback receipts"
    ],
}
with open("REPORT.json", "w", encoding="utf-8") as f:
    json.dump(report, f, indent=2, sort_keys=True)
    f.write("\n")
PY
}

write_skipped_report() {
  reason="$1"
  python3 - "$reason" "$max_fix_attempts" <<'PY'
import json
import os
import sys

reason, max_fix_attempts = sys.argv[1], int(sys.argv[2])
report = {
    "schema": "bb.incident_triage_response.v1",
    "status": "skipped_escalated",
    "bb_run_id": os.environ.get("BB_TRIAGE_RUN_ID", ""),
    "delivery_id": os.environ.get("BB_TRIAGE_DELIVERY_ID", ""),
    "incident": {
        "id": os.environ.get("BB_TRIAGE_INCIDENT_ID", ""),
        "service": os.environ.get("BB_TRIAGE_SERVICE", ""),
        "severity": os.environ.get("BB_TRIAGE_SEVERITY", ""),
        "fingerprint": os.environ.get("BB_TRIAGE_FINGERPRINT", ""),
    },
    "repo": os.environ.get("BB_TRIAGE_REPO", ""),
    "progress_writebacks": [],
    "hypotheses": [],
    "experiments": [],
    "fix_attempts": [],
    "iteration_guard": {
        "max_fix_attempts": max_fix_attempts,
        "attempts_used": 0,
        "stopped": False,
        "reason": None,
    },
    "scope_honesty": {
        "auto_deploy_on_merge": os.environ.get("BB_TRIAGE_AUTO_DEPLOY_ON_MERGE") == "true",
        "v1_stop": "skipped_already_escalated",
    },
    "escalation": {"escalated": True, "reason": reason, "response": None},
    "artifact_paths": ["REPORT.json"],
    "residual_risk": [reason],
}
with open("REPORT.json", "w", encoding="utf-8") as f:
    json.dump(report, f, indent=2, sort_keys=True)
    f.write("\n")
print(json.dumps({
    "schema_version": "bb.command_result.v1",
    "result": f"incident triage skipped: {reason}",
}, sort_keys=True))
PY
}

if [ "$BB_TRIAGE_ALLOWED" != "1" ]; then
  write_blocked_report "service is not in the incident-triage whitelist"
  exit 0
fi

if [ -z "${OPENROUTER_API_KEY:-}" ]; then
  write_blocked_report "OPENROUTER_API_KEY is unset"
  exit 0
fi
if [ -z "${GH_TOKEN:-}" ]; then
  write_blocked_report "GH_TOKEN is unset"
  exit 0
fi
if [ -z "${CANARY_ENDPOINT:-}" ] || [ -z "${CANARY_API_KEY:-}" ]; then
  write_blocked_report "CANARY_ENDPOINT or CANARY_API_KEY is unset"
  exit 0
fi

# Escalated incidents are never worked by triage agents. Canary does not yet
# guarantee an escalation flag on the webhook payload itself (tracked
# cross-repo), so this preflight asks Canary directly. Network failure or an
# absent field fails open (proceed with triage) rather than silently
# dropping incidents before Canary ships the field.
if [ -n "$BB_TRIAGE_INCIDENT_ID" ]; then
  already_escalated="false"
  if curl -fsS "$CANARY_ENDPOINT/api/v1/incidents/$BB_TRIAGE_INCIDENT_ID" \
      -H "Authorization: Bearer $CANARY_API_KEY" \
      -o .bb-incident-preflight.json 2>/dev/null; then
    already_escalated=$(python3 - .bb-incident-preflight.json <<'PY'
import json
import sys

try:
    with open(sys.argv[1], "r", encoding="utf-8") as f:
        data = json.load(f)
except (OSError, ValueError):
    print("false")
    raise SystemExit(0)
incident = data.get("incident")
if not isinstance(incident, dict):
    incident = data if isinstance(data, dict) else {}
escalated_at = incident.get("escalated_at")
is_escalated = incident.get("is_escalated")
print("true" if (escalated_at or is_escalated) else "false")
PY
)
  fi
  rm -f .bb-incident-preflight.json
  if [ "$already_escalated" = "true" ]; then
    write_skipped_report "incident $BB_TRIAGE_INCIDENT_ID is already escalated; escalated incidents are never worked by triage agents"
    exit 0
  fi
fi

agent_cmd=""
agent_via_npx="0"
if agent_cmd="$(command -v "$agent_bin" 2>/dev/null)"; then
  :
elif [ "$agent_bin" = "pi" ] && agent_cmd="$(command -v npx 2>/dev/null)"; then
  agent_via_npx="1"
else
  write_blocked_report "agent binary '$agent_bin' not found and no supported fallback is available"
  exit 0
fi

mkdir -p "$agent_out_dir"
cat > "$prompt_file" <<EOF
Read LANE_CARD.md first. It is the operator contract.

Wrapper-verified context:
- bb_run_id: $BB_TRIAGE_RUN_ID
- delivery_id: $BB_TRIAGE_DELIVERY_ID
- incident_id: $BB_TRIAGE_INCIDENT_ID
- service: $BB_TRIAGE_SERVICE
- severity: $BB_TRIAGE_SEVERITY
- fingerprint: $BB_TRIAGE_FINGERPRINT
- repo: $BB_TRIAGE_REPO
- repo_dir: $BB_TRIAGE_REPO_DIR
- auto_deploy_on_merge: $BB_TRIAGE_AUTO_DEPLOY_ON_MERGE
- max_fix_attempts: $max_fix_attempts

You must write REPORT.json at workspace root before exiting. Do not include
secret values in REPORT.json, stdout, stderr, git remotes, PR bodies, or
Canary writebacks.
EOF

printf '\nEVENT.json:\n\n' >> "$prompt_file"
cat "$event_file" >> "$prompt_file"
printf '\n\nRUN.json:\n\n' >> "$prompt_file"
cat "$run_file" >> "$prompt_file"

run_agent() {
  if [ "$agent_via_npx" = "1" ]; then
    "$agent_cmd" \
      -y \
      "$agent_npx_package" \
      --provider "$provider" \
      --model "$model" \
      --thinking "$agent_thinking" \
      --no-session \
      --mode json \
      -p
  elif [ "$agent_bin" = "pi" ]; then
    "$agent_cmd" \
      --provider "$provider" \
      --model "$model" \
      --thinking "$agent_thinking" \
      --no-session \
      --mode json \
      -p
  else
    "$agent_cmd" \
      --provider "$provider" \
      --model "$model" \
      --no-session \
      --mode json \
      -p
  fi
}

stdout_pipe="$agent_out_dir/stdout.pipe"
stdout_cap_marker="$agent_out_dir/stdout.cap"
rm -f "$stdout_pipe" "$stdout_cap_marker" "$agent_out_dir/stdout.jsonl"
mkfifo "$stdout_pipe"
python3 - "$agent_stdout_max_bytes" "$stdout_pipe" "$agent_out_dir/stdout.jsonl" "$stdout_cap_marker" <<'PY' &
import sys

limit_raw, pipe_path, stdout_path, marker_path = sys.argv[1:5]
limit = int(limit_raw)
written = 0
with open(pipe_path, "rb", buffering=0) as src, open(stdout_path, "wb") as dst:
    while True:
        chunk = src.read(8192)
        if not chunk:
            break
        remaining = limit - written
        if remaining > 0:
            piece = chunk[:remaining]
            dst.write(piece)
            written += len(piece)
        if len(chunk) > remaining:
            with open(marker_path, "w", encoding="utf-8") as f:
                f.write(str(written))
            raise SystemExit(153)
PY
cap_pid=$!
set +e
run_agent < "$prompt_file" > "$stdout_pipe" 2> "$agent_out_dir/stderr.txt"
agent_status=$?
wait "$cap_pid"
cap_status=$?
set -e
rm -f "$stdout_pipe"
if [ -f "$stdout_cap_marker" ]; then
  printf 'agent stdout exceeded INCIDENT_TRIAGE_AGENT_STDOUT_MAX_BYTES=%s\n' "$agent_stdout_max_bytes" >> "$agent_out_dir/stderr.txt"
  agent_status=153
  rm -f "$stdout_cap_marker"
elif [ "$cap_status" -ne 0 ] && [ "$agent_status" -eq 0 ]; then
  agent_status="$cap_status"
fi

if [ "$agent_status" -ne 0 ] && [ ! -f REPORT.json ]; then
  if write_report_from_terminal_writebacks "agent command exited $agent_status before REPORT.json"; then
    printf 'agent command exited %s before REPORT.json; synthesized REPORT.json from Canary writeback receipts\n' "$agent_status" >> "$agent_out_dir/stderr.txt"
  else
    write_blocked_report "agent command exited $agent_status before REPORT.json"
    exit 0
  fi
fi
if [ "$agent_status" -ne 0 ]; then
  printf 'agent command exited %s after REPORT.json\n' "$agent_status" >> "$agent_out_dir/stderr.txt"
fi

python3 - "$agent_out_dir/stdout.jsonl" "$max_fix_attempts" "$BB_TRIAGE_REPO" <<'PY'
import json
import sys

stdout_path, max_fix_attempts_raw, expected_repo = sys.argv[1:4]
max_fix_attempts = int(max_fix_attempts_raw)

try:
    with open("REPORT.json", "r", encoding="utf-8") as f:
        report = json.load(f)
except FileNotFoundError:
    raise SystemExit("REPORT.json missing after incident triage agent")

if report.get("schema") != "bb.incident_triage_response.v1":
    raise SystemExit("REPORT.json has wrong schema")
if report.get("repo") != expected_repo:
    raise SystemExit(f"REPORT.json repo mismatch: {report.get('repo')} != {expected_repo}")
guard = report.get("iteration_guard") or {}
if int(guard.get("max_fix_attempts", -1)) != max_fix_attempts:
    raise SystemExit("REPORT.json iteration_guard.max_fix_attempts mismatch")
attempts_used = int(guard.get("attempts_used", len(report.get("fix_attempts") or [])))
if attempts_used > max_fix_attempts:
    raise SystemExit("REPORT.json exceeds max fix attempts")
if report.get("artifact_paths") != ["REPORT.json"]:
    raise SystemExit('REPORT.json artifact_paths must equal ["REPORT.json"]')

tokens_in = tokens_out = turns = 0
cost = 0.0
saw_usage = False
try:
    lines = open(stdout_path, "r", encoding="utf-8").read().splitlines()
except FileNotFoundError:
    lines = []
for line in lines:
    try:
        value = json.loads(line)
    except json.JSONDecodeError:
        continue
    if value.get("type") == "turn_end":
        turns += 1
    if value.get("type") != "message_end":
        continue
    message = value.get("message") or {}
    if message.get("role") != "assistant":
        continue
    usage = message.get("usage") or {}
    if usage:
        saw_usage = True
    tokens_in += int(usage.get("input") or 0)
    tokens_out += int(usage.get("output") or 0)
    cost += float((usage.get("cost") or {}).get("total") or 0.0)

incident = report.get("incident") or {}
result = {
    "schema_version": "bb.command_result.v1",
    "result": "incident triage {status} for {repo} {incident}".format(
        status=report.get("status", "unknown"),
        repo=expected_repo,
        incident=incident.get("id", ""),
    ),
    "tokens_in": tokens_in if saw_usage else None,
    "tokens_out": tokens_out if saw_usage else None,
    "turns": turns or None,
    "cost_usd": cost if saw_usage else None,
}
print(json.dumps(result, sort_keys=True))
PY

# Mechanical iteration-guard backstop. The agent self-reports fix attempts in
# REPORT.json, but nothing above this line independently proves it stayed
# inside max_fix_attempts, and a report that forgets to call Canary's
# /escalate is silently unbounded. Cross-check attempts_used against the
# actual bb/incident-<id>-attempt-<n> branches in the checked-out repo (the
# one artifact the agent cannot under-report without lying to git itself),
# escalate on Canary as a backstop when the guard fired but the report did
# not already escalate, and hard-fail the run when the repo proves the
# agent exceeded max_fix_attempts regardless of what it claimed.
python3 - "$max_fix_attempts" "$BB_TRIAGE_REPO_DIR" "$BB_TRIAGE_INCIDENT_ID" \
  > .bb-escalate-plan.json <<'PY'
import json
import os
import subprocess
import sys

max_fix_attempts, repo_dir, incident_id = sys.argv[1], sys.argv[2], sys.argv[3]
max_fix_attempts = int(max_fix_attempts)

with open("REPORT.json", "r", encoding="utf-8") as f:
    report = json.load(f)

guard = report.get("iteration_guard") or {}
reported_attempts = int(guard.get("attempts_used", len(report.get("fix_attempts") or [])))

branch_attempts = set()
if incident_id and repo_dir and os.path.isdir(os.path.join(repo_dir, ".git")):
    prefix = f"bb/incident-{incident_id}-attempt-"
    try:
        out = subprocess.run(
            [
                "git", "-C", repo_dir, "for-each-ref",
                "--format=%(refname:short)", "refs/heads", "refs/remotes",
            ],
            capture_output=True, text=True, timeout=30, check=True,
        ).stdout
    except (OSError, subprocess.SubprocessError) as exc:
        print(f"branch verification skipped: {exc}", file=sys.stderr)
        out = ""
    for line in out.splitlines():
        short = line.split("/", 1)[1] if line.startswith("origin/") else line
        if prefix in short:
            suffix = short.rsplit("-attempt-", 1)[-1]
            if suffix.isdigit():
                branch_attempts.add(int(suffix))

verified_attempts = max(reported_attempts, len(branch_attempts))
mismatch = len(branch_attempts) > reported_attempts
overrun = len(branch_attempts) > max_fix_attempts
self_flagged_stop = bool(guard.get("stopped")) and report.get("status") == "escalation_needed"
already_escalated = bool((report.get("escalation") or {}).get("escalated"))
should_escalate = (self_flagged_stop or overrun) and not already_escalated

plan = {
    "verified_attempts": verified_attempts,
    "branch_attempts": sorted(branch_attempts),
    "mismatch": mismatch,
    "overrun": overrun,
    "should_escalate": should_escalate,
    "reason": guard.get("reason")
    or (
        f"mechanical check found {len(branch_attempts)} attempt branches "
        f"exceeding max_fix_attempts {max_fix_attempts}"
        if overrun
        else "iteration guard exhausted"
    ),
}
print(json.dumps(plan))

report.setdefault("iteration_guard", {})["attempts_used"] = verified_attempts
if mismatch:
    report["residual_risk"] = list(report.get("residual_risk") or []) + [
        f"mechanical branch check found {len(branch_attempts)} attempt branches "
        f"{sorted(branch_attempts)} vs reported attempts_used {reported_attempts}"
    ]
with open("REPORT.json", "w", encoding="utf-8") as f:
    json.dump(report, f, indent=2, sort_keys=True)
    f.write("\n")
PY

should_escalate=$(python3 -c "import json; print(json.load(open('.bb-escalate-plan.json'))['should_escalate'])")
overrun=$(python3 -c "import json; print(json.load(open('.bb-escalate-plan.json'))['overrun'])")

if [ "$should_escalate" = "True" ]; then
  escalate_reason=$(python3 -c "import json; print(json.load(open('.bb-escalate-plan.json'))['reason'])")
  ESCALATE_REASON="$escalate_reason" python3 - "$BB_TRIAGE_RUN_ID" "$BB_TRIAGE_INCIDENT_ID" \
    > .bb-escalate-request.json <<'PY'
import json
import os
import sys

run_id, incident_id = sys.argv[1], sys.argv[2]
print(json.dumps({
    "reason": os.environ["ESCALATE_REASON"],
    "owner": "bitterblossom/canary-triage",
    "purpose": "triage_escalation",
    "idempotency_key": f"bb-run-{run_id}:{incident_id}:escalate",
}))
PY
  escalate_status=0
  curl -fsS -X POST "$CANARY_ENDPOINT/api/v1/incidents/$BB_TRIAGE_INCIDENT_ID/escalate" \
    -H "Authorization: Bearer $CANARY_API_KEY" \
    -H "Content-Type: application/json" \
    -d @.bb-escalate-request.json \
    -o .bb-escalate-response.json 2>.bb-escalate-error.txt || escalate_status=$?
  python3 - "$escalate_status" <<'PY'
import json
import sys

status = int(sys.argv[1])
with open("REPORT.json", "r", encoding="utf-8") as f:
    report = json.load(f)
with open(".bb-escalate-request.json", "r", encoding="utf-8") as f:
    reason = json.load(f).get("reason")
if status == 0:
    try:
        with open(".bb-escalate-response.json", "r", encoding="utf-8") as f:
            response = json.load(f)
    except (OSError, ValueError):
        response = None
    report["escalation"] = {"escalated": True, "reason": reason, "response": response}
else:
    try:
        with open(".bb-escalate-error.txt", "r", encoding="utf-8") as f:
            err = f.read().strip()
    except OSError:
        err = ""
    report["escalation"] = {
        "escalated": False,
        "reason": reason,
        "response": {"error": err or f"curl exited {status}"},
    }
with open("REPORT.json", "w", encoding="utf-8") as f:
    json.dump(report, f, indent=2, sort_keys=True)
    f.write("\n")
PY
  rm -f .bb-escalate-request.json .bb-escalate-response.json .bb-escalate-error.txt
fi

if [ "$overrun" = "True" ]; then
  branches=$(python3 -c "import json; print(json.load(open('.bb-escalate-plan.json'))['branch_attempts'])")
  rm -f .bb-escalate-plan.json
  echo "iteration guard violated: attempt branches $branches in $BB_TRIAGE_REPO_DIR exceed max_fix_attempts $max_fix_attempts" >&2
  exit 1
fi
rm -f .bb-escalate-plan.json
