#!/bin/sh
# Supervises bb serve with Litestream replication when a replica env is present.
set -eu

log() {
  printf '%s\n' "$*" >&2
}

fail() {
  log "bb-litestream-entrypoint: $*"
  exit 2
}

is_truthy() {
  case "${1:-}" in
    1|true|TRUE|yes|YES|on|ON) return 0 ;;
    *) return 1 ;;
  esac
}

valid_env_name() {
  case "$1" in
    ""|[0-9]*|*[!ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789_]*)
      return 1
      ;;
    *)
      return 0
      ;;
  esac
}

require_absolute_path() {
  label=$1
  value=$2
  case "$value" in
    /*) ;;
    *) fail "$label must be an absolute path: $value" ;;
  esac
}

if [ "$#" -eq 0 ]; then
  set -- bb serve
fi

plane_dir=${BB_PLANE_DIR:-/app/plane}
db_path=${BB_LITESTREAM_DB_PATH:-"$plane_dir/.bb/plane.db"}
config_path=${BB_LITESTREAM_CONFIG:-/tmp/bb-litestream.yml}
replica_env_name=${BB_LITESTREAM_REPLICA_URL_ENV:-LITESTREAM_REPLICA_URL}
heartbeat_path=${BB_LITESTREAM_HEARTBEAT_PATH:-"$plane_dir/.bb/backup-last-success"}
socket_path=${BB_LITESTREAM_SOCKET_PATH:-/tmp/bb-litestream.sock}
sync_interval=${BB_LITESTREAM_SYNC_INTERVAL_SECONDS:-60}
sync_timeout=${BB_LITESTREAM_SYNC_TIMEOUT_SECONDS:-30}
startup_timeout=${BB_LITESTREAM_STARTUP_TIMEOUT_SECONDS:-60}

valid_env_name "$replica_env_name" || fail "BB_LITESTREAM_REPLICA_URL_ENV must name one environment variable"
require_absolute_path "BB_LITESTREAM_DB_PATH" "$db_path"
require_absolute_path "BB_LITESTREAM_CONFIG" "$config_path"
require_absolute_path "BB_LITESTREAM_HEARTBEAT_PATH" "$heartbeat_path"
require_absolute_path "BB_LITESTREAM_SOCKET_PATH" "$socket_path"

eval "replica_url=\${$replica_env_name:-}"
if [ -z "$replica_url" ]; then
  if is_truthy "${BB_LITESTREAM_REQUIRED:-0}"; then
    fail "$replica_env_name is required because BB_LITESTREAM_REQUIRED=1"
  fi
  log "bb-litestream-entrypoint: $replica_env_name is unset; starting without Litestream"
  exec "$@"
fi

mkdir -p "$(dirname "$db_path")" "$(dirname "$config_path")" "$(dirname "$heartbeat_path")" "$(dirname "$socket_path")"
rm -f "$socket_path"

# Ephemeral-disk substrates (DO App Platform) have no volume carrying the
# plane's workload config (plane.toml, agents/, tasks/). When
# BB_PLANE_CONFIG_URL is set and config is absent, fetch and unpack it before
# anything touches the plane dir. The URL is typically a presigned
# object-store link: treat it as a secret, never log it.
if [ -n "${BB_PLANE_CONFIG_URL:-}" ] && [ ! -f "$plane_dir/plane.toml" ]; then
  log "bb-litestream-entrypoint: plane config missing; fetching bootstrap archive"
  curl -fsSL "$BB_PLANE_CONFIG_URL" | tar xz -C "$plane_dir" \
    || fail "plane config bootstrap fetch failed"
  [ -f "$plane_dir/plane.toml" ] || fail "plane config bootstrap did not materialize plane.toml"
fi

{
  printf '%s\n' 'socket:'
  printf '%s\n' '  enabled: true'
  printf '  path: %s\n' "$socket_path"
  printf '%s\n' 'dbs:'
  printf '  - path: %s\n' "$db_path"
  printf '%s\n' '    replica:'
  # shellcheck disable=SC2016 # Keep the secret as a Litestream env reference.
  printf '      url: ${%s}\n' "$replica_env_name"
} >"$config_path"

if [ ! -f "$db_path" ]; then
  log "bb-litestream-entrypoint: database missing; attempting Litestream restore"
  litestream restore -if-replica-exists -o "$db_path" -config "$config_path" "$db_path" >/dev/null 2>&1 \
    || fail "Litestream restore failed for missing database"
fi

if [ "$1" = "bb" ] && [ "${2:-}" = "serve" ]; then
  # Create the SQLite ledger after any replica restore opportunity.
  BB_PLANE_DIR="$plane_dir" "$1" status --json >/dev/null
fi

litestream replicate -config "$config_path" >/dev/null 2>&1 &
litestream_pid=$!

sync_once() {
  if litestream sync -socket "$socket_path" -wait -timeout "$sync_timeout" "$db_path" >/dev/null 2>&1; then
    date -u '+%Y-%m-%dT%H:%M:%SZ' >"$heartbeat_path"
    return 0
  fi
  return 1
}

startup_deadline=$(( $(date +%s) + startup_timeout ))
while ! sync_once; do
  if ! kill -0 "$litestream_pid" 2>/dev/null; then
    set +e
    wait "$litestream_pid"
    litestream_status=$?
    set -e
    if [ "$litestream_status" -eq 0 ]; then
      litestream_status=1
    fi
    log "bb-litestream-entrypoint: litestream exited before initial sync"
    exit "$litestream_status"
  fi
  if [ "$(date +%s)" -ge "$startup_deadline" ]; then
    log "bb-litestream-entrypoint: initial Litestream sync did not complete within ${startup_timeout}s"
    kill "$litestream_pid" 2>/dev/null || true
    wait "$litestream_pid" 2>/dev/null || true
    exit 2
  fi
  sleep 1
done

heartbeat_loop() {
  while :; do
    sync_once || true
    if [ "${BB_TEST_ENTRYPOINT_ONCE:-0}" = "1" ]; then
      break
    fi
    sleep "$sync_interval"
  done
}

heartbeat_loop &
heartbeat_pid=$!

"$@" &
app_pid=$!

cleanup() {
  kill "$heartbeat_pid" "$litestream_pid" "$app_pid" 2>/dev/null || true
  wait "$heartbeat_pid" "$litestream_pid" "$app_pid" 2>/dev/null || true
}

trap 'cleanup; exit 143' INT TERM

while :; do
  if ! kill -0 "$app_pid" 2>/dev/null; then
    set +e
    wait "$app_pid"
    status=$?
    set -e
    cleanup
    exit "$status"
  fi
  if ! kill -0 "$litestream_pid" 2>/dev/null; then
    set +e
    wait "$litestream_pid"
    litestream_status=$?
    set -e
    if [ "$litestream_status" -eq 0 ]; then
      litestream_status=1
    fi
    log "bb-litestream-entrypoint: litestream exited; stopping bb"
    cleanup
    exit "$litestream_status"
  fi
  sleep 1
done
