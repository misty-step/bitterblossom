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

if [ "$agent_via_npx" = "1" ]; then
  "$agent_cmd" \
    -y \
    "$agent_npx_package" \
    --provider "$provider" \
    --model "$model" \
    --no-session \
    --mode json \
    -p < "$prompt_file" > "$agent_out_dir/stdout.jsonl" 2> "$agent_out_dir/stderr.txt"
else
  "$agent_cmd" \
    --provider "$provider" \
    --model "$model" \
    --no-session \
    --mode json \
    -p < "$prompt_file" > "$agent_out_dir/stdout.jsonl" 2> "$agent_out_dir/stderr.txt"
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
