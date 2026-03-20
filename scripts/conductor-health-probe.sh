#!/usr/bin/env bash
set -euo pipefail

health_url="${1:-${CONDUCTOR_HEALTHCHECK_URL:-}}"

if [[ -z "${health_url}" ]]; then
  port="${CONDUCTOR_HEALTH_PORT:-4000}"
  health_url="http://127.0.0.1:${port}/healthz"
fi

timeout_seconds="${CONDUCTOR_HEALTH_TIMEOUT_SECONDS:-5}"
timestamp="$(date -u +"%Y-%m-%dT%H:%M:%SZ")"

if curl --fail --silent --show-error --max-time "${timeout_seconds}" "${health_url}" >/dev/null; then
  echo "ok ${timestamp} ${health_url}"
  exit 0
fi

message="bitterblossom conductor health check failed at ${timestamp}: ${health_url}"

if [[ -n "${CONDUCTOR_ALERT_WEBHOOK:-}" ]]; then
  payload="$(
    ALERT_MESSAGE="${message}" python3 -c 'import json, os; print(json.dumps({"text": os.environ["ALERT_MESSAGE"]}))'
  )"

  curl \
    --fail --silent --show-error \
    -X POST \
    -H "content-type: application/json" \
    --data "${payload}" \
    "${CONDUCTOR_ALERT_WEBHOOK}" >/dev/null || true
fi

echo "${message}" >&2
exit 1
