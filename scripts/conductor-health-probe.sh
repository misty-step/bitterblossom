#!/usr/bin/env bash
set -euo pipefail

json_escape() {
  local input="$1"
  local output=""
  local char=""
  local code=0
  local i=0
  local original_lc_all="${LC_ALL-}"

  LC_ALL=C

  for ((i = 0; i < ${#input}; i++)); do
    char="${input:i:1}"

    case "${char}" in
      '"')
        output+='\"'
        ;;
      "\\")
        output+='\\'
        ;;
      $'\b')
        output+='\b'
        ;;
      $'\f')
        output+='\f'
        ;;
      $'\n')
        output+='\n'
        ;;
      $'\r')
        output+='\r'
        ;;
      $'\t')
        output+='\t'
        ;;
      *)
        printf -v code '%d' "'${char}"

        if ((code < 32)); then
          printf -v char '\\u%04x' "${code}"
          output+="${char}"
        else
          output+="${char}"
        fi
        ;;
    esac
  done

  if [[ -n "${original_lc_all}" ]]; then
    LC_ALL="${original_lc_all}"
  else
    unset LC_ALL
  fi

  printf '%s' "${output}"
}

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
  payload="{\"text\":\"$(json_escape "${message}")\"}"

  curl \
    --fail --silent --show-error \
    -X POST \
    -H "content-type: application/json" \
    --data "${payload}" \
    "${CONDUCTOR_ALERT_WEBHOOK}" >/dev/null || true
fi

echo "${message}" >&2
exit 1
