#!/bin/sh
# Register an external local dispatch with a running Bitterblossom plane.
# Fire-and-forget by design: registration must never block the dispatch itself.
set -u

case "${1:-start}" in
  start|register|running|done|failed|update) ;;
  *) exit 0 ;;
esac

[ -n "${BB_URL:-}" ] || exit 0
[ -n "${BB_API_TOKEN:-}" ] || exit 0
command -v curl >/dev/null 2>&1 || exit 0
command -v python3 >/dev/null 2>&1 || exit 0

base_url=${BB_URL%/}
timeout=${BB_REGISTER_TIMEOUT_SEC:-2}
tmp_dir=${TMPDIR:-/tmp}
payload=$(mktemp "$tmp_dir/bb-register-payload.XXXXXX") || exit 0
response=$(mktemp "$tmp_dir/bb-register-response.XXXXXX") || {
  rm -f "$payload"
  exit 0
}
cleanup() {
  rm -f "$payload" "$response"
}
trap cleanup EXIT INT TERM

utc_now() {
  date -u '+%Y-%m-%dT%H:%M:%SZ'
}

curl_config() {
  method=$1
  url=$2
  {
    printf '%s\n' 'silent'
    printf 'max-time = %s\n' "$timeout"
    printf 'request = "%s"\n' "$method"
    printf 'url = "%s"\n' "$url"
    printf 'header = "Authorization: Bearer %s"\n' "$BB_API_TOKEN"
    printf '%s\n' 'header = "Content-Type: application/json"'
  }
}

write_start_payload() {
  BB_REGISTER_AGENT=${BB_REGISTER_AGENT:-${BB_AGENT:-${USER:-external}}} \
  BB_REGISTER_ROLE=${BB_REGISTER_ROLE:-external} \
  BB_REGISTER_REPO=${BB_REGISTER_REPO:-${PWD##*/}} \
  BB_REGISTER_BRIEF_HASH=${BB_REGISTER_BRIEF_HASH:-unknown} \
  BB_REGISTER_PLANE=${BB_REGISTER_PLANE:-local} \
  BB_REGISTER_STATUS_URL=${BB_REGISTER_STATUS_URL:-} \
  BB_REGISTER_RECEIPT_PATH=${BB_REGISTER_RECEIPT_PATH:-} \
  BB_REGISTER_STARTED_AT=${BB_REGISTER_STARTED_AT:-$(utc_now)} \
  python3 - "$payload" <<'PY'
import json, os, sys

payload = {
    "agent": os.environ["BB_REGISTER_AGENT"],
    "role": os.environ["BB_REGISTER_ROLE"],
    "repo": os.environ["BB_REGISTER_REPO"],
    "brief_hash": os.environ["BB_REGISTER_BRIEF_HASH"],
    "plane": os.environ["BB_REGISTER_PLANE"],
    "started_at": os.environ["BB_REGISTER_STARTED_AT"],
}
for field, env in [("status_url", "BB_REGISTER_STATUS_URL"), ("receipt_path", "BB_REGISTER_RECEIPT_PATH")]:
    value = os.environ.get(env, "").strip()
    if value:
        payload[field] = value
with open(sys.argv[1], "w", encoding="utf-8") as handle:
    json.dump(payload, handle, separators=(",", ":"))
PY
}

write_patch_payload() {
  status=$1
  completed=${BB_REGISTER_COMPLETED_AT:-}
  if [ "$status" = "done" ] || [ "$status" = "failed" ]; then
    completed=${completed:-$(utc_now)}
  fi
  BB_REGISTER_STATUS=$status \
  BB_REGISTER_COMPLETED_AT=$completed \
  python3 - "$payload" <<'PY'
import json, os, sys

payload = {"status": os.environ["BB_REGISTER_STATUS"]}
completed = os.environ.get("BB_REGISTER_COMPLETED_AT", "").strip()
if completed:
    payload["completed_at"] = completed
with open(sys.argv[1], "w", encoding="utf-8") as handle:
    json.dump(payload, handle, separators=(",", ":"))
PY
}

post_start() {
  write_start_payload >/dev/null 2>&1 || exit 0
  curl_config POST "$base_url/api/external-runs" \
    | curl --config - --data-binary "@$payload" -o "$response" >/dev/null 2>&1 || exit 0
  [ "${BB_REGISTER_PRINT_ID:-1}" = "1" ] || exit 0
  python3 - "$response" <<'PY' 2>/dev/null || true
import json, sys
with open(sys.argv[1], encoding="utf-8") as handle:
    value = json.load(handle)
run_id = value.get("id")
if run_id:
    print(run_id)
PY
}

patch_status() {
  status=$1
  id=${2:-${BB_REGISTER_ID:-}}
  [ -n "$id" ] || exit 0
  write_patch_payload "$status" >/dev/null 2>&1 || exit 0
  curl_config PATCH "$base_url/api/external-runs/$id" \
    | curl --config - --data-binary "@$payload" -o "$response" >/dev/null 2>&1 || exit 0
}

case "${1:-start}" in
  start|register) post_start ;;
  update) patch_status "${2:-${BB_REGISTER_STATUS:-running}}" "${3:-${BB_REGISTER_ID:-}}" ;;
  running|done|failed) patch_status "$1" "${2:-${BB_REGISTER_ID:-}}" ;;
esac
