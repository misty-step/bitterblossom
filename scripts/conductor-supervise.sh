#!/usr/bin/env bash

set -euo pipefail

STATE_DIR="${BB_CONDUCTOR_STATE_DIR:-$HOME/.bb/conductor-supervisor}"
LOG_FILE="$STATE_DIR/current.log"
SUPERVISOR_PID_FILE="$STATE_DIR/supervisor.pid"
CHILD_PID_FILE="$STATE_DIR/child.pid"
LOCK_DIR="$STATE_DIR/lock"
LAUNCHER_FILE="$STATE_DIR/launch.sh"
LOG_MAX_BYTES="${BB_CONDUCTOR_LOG_MAX_BYTES:-10485760}"
LOG_KEEP_FILES="${BB_CONDUCTOR_LOG_KEEP_FILES:-10}"
RESTART_DELAY_SECONDS="${BB_CONDUCTOR_RESTART_DELAY_SECONDS:-5}"

mkdir -p "$STATE_DIR"

timestamp() {
  date +"%Y-%m-%d %H:%M:%S"
}

usage() {
  cat <<'EOF'
Usage:
  scripts/conductor-supervise.sh start [conductor loop args...]
  scripts/conductor-supervise.sh run [conductor loop args...]
  scripts/conductor-supervise.sh stop
  scripts/conductor-supervise.sh status
  scripts/conductor-supervise.sh install-cron [--repo-root PATH] [conductor loop args...]
  scripts/conductor-supervise.sh rotate-logs [--force]

Examples:
  scripts/conductor-supervise.sh start --repo misty-step/bitterblossom --label autopilot --worker noble-blue-serpent
  scripts/conductor-supervise.sh install-cron --repo-root /home/sprite/workspace/bitterblossom --repo misty-step/bitterblossom --label autopilot --worker noble-blue-serpent
EOF
}

append_log() {
  local line="$1"
  rotate_logs_if_needed false
  printf '[%s] %s\n' "$(timestamp)" "$line" >> "$LOG_FILE"
}

rotate_logs_if_needed() {
  local force="${1:-false}"
  if [[ ! -f "$LOG_FILE" ]]; then
    return 0
  fi

  local size
  size=$(wc -c < "$LOG_FILE")
  if [[ "$force" != "true" ]] && (( size < LOG_MAX_BYTES )); then
    return 0
  fi

  local archive="$STATE_DIR/conductor-$(date +"%Y%m%d-%H%M%S")"
  if [[ -e "${archive}.log" ]]; then
    archive="${archive}-$$"
  fi
  mv "$LOG_FILE" "${archive}.log"

  local archived_logs=()
  while IFS= read -r archived_log; do
    archived_logs+=("$archived_log")
  done < <(find "$STATE_DIR" -maxdepth 1 -type f -name 'conductor-*.log' | sort)
  local total="${#archived_logs[@]}"
  if (( total <= LOG_KEEP_FILES )); then
    return 0
  fi

  local remove_count=$(( total - LOG_KEEP_FILES ))
  local index
  for (( index=0; index<remove_count; index++ )); do
    rm -f "${archived_logs[$index]}"
  done
}

ensure_not_running() {
  if [[ -f "$SUPERVISOR_PID_FILE" ]]; then
    local existing_pid
    existing_pid="$(cat "$SUPERVISOR_PID_FILE")"
    if kill -0 "$existing_pid" 2>/dev/null; then
      echo "supervisor already running with pid $existing_pid" >&2
      exit 1
    fi
    rm -f "$SUPERVISOR_PID_FILE"
  fi
}

acquire_lock() {
  if mkdir "$LOCK_DIR" 2>/dev/null; then
    return 0
  fi

  if [[ -f "$SUPERVISOR_PID_FILE" ]]; then
    local existing_pid
    existing_pid="$(cat "$SUPERVISOR_PID_FILE")"
    if kill -0 "$existing_pid" 2>/dev/null; then
      echo "supervisor already running with pid $existing_pid" >&2
      exit 1
    fi
  fi

  rm -rf "$LOCK_DIR"
  mkdir "$LOCK_DIR"
}

cleanup_supervisor() {
  if [[ -f "$CHILD_PID_FILE" ]]; then
    local child_pid
    child_pid="$(cat "$CHILD_PID_FILE")"
    if kill -0 "$child_pid" 2>/dev/null; then
      kill "$child_pid" 2>/dev/null || true
      wait "$child_pid" 2>/dev/null || true
    fi
  fi
  rm -f "$SUPERVISOR_PID_FILE" "$CHILD_PID_FILE"
  rm -rf "$LOCK_DIR"
}

stop_supervisor() {
  if [[ ! -f "$SUPERVISOR_PID_FILE" ]]; then
    echo "supervisor not running"
    return 0
  fi

  local supervisor_pid
  supervisor_pid="$(cat "$SUPERVISOR_PID_FILE")"
  if ! kill -0 "$supervisor_pid" 2>/dev/null; then
    echo "supervisor pid file is stale"
    rm -f "$SUPERVISOR_PID_FILE" "$CHILD_PID_FILE"
    return 0
  fi

  if [[ -f "$CHILD_PID_FILE" ]]; then
    local child_pid
    child_pid="$(cat "$CHILD_PID_FILE")"
    if kill -0 "$child_pid" 2>/dev/null; then
      kill "$child_pid" 2>/dev/null || true
    fi
  fi

  kill "$supervisor_pid"
  echo "stopped supervisor pid $supervisor_pid"
}

status_supervisor() {
  local supervisor_status="stopped"
  local supervisor_pid=""
  local child_status="stopped"
  local child_pid=""

  if [[ -f "$SUPERVISOR_PID_FILE" ]]; then
    supervisor_pid="$(cat "$SUPERVISOR_PID_FILE")"
    if kill -0 "$supervisor_pid" 2>/dev/null; then
      supervisor_status="running"
    fi
  fi

  if [[ -f "$CHILD_PID_FILE" ]]; then
    child_pid="$(cat "$CHILD_PID_FILE")"
    if kill -0 "$child_pid" 2>/dev/null; then
      child_status="running"
    fi
  fi

  printf 'state_dir=%s\n' "$STATE_DIR"
  printf 'log_file=%s\n' "$LOG_FILE"
  printf 'launcher=%s\n' "$LAUNCHER_FILE"
  printf 'supervisor_status=%s\n' "$supervisor_status"
  [[ -n "$supervisor_pid" ]] && printf 'supervisor_pid=%s\n' "$supervisor_pid"
  printf 'child_status=%s\n' "$child_status"
  [[ -n "$child_pid" ]] && printf 'child_pid=%s\n' "$child_pid"

  if crontab -l 2>/dev/null | grep -F "$LAUNCHER_FILE" >/dev/null 2>&1; then
    echo "cron_status=installed"
  else
    echo "cron_status=missing"
  fi
}

write_launcher() {
  local repo_root="$1"
  shift

  {
    echo '#!/usr/bin/env bash'
    echo 'set -euo pipefail'
    printf 'cd %q\n' "$repo_root"
    printf 'exec %q run' "$repo_root/scripts/conductor-supervise.sh"
    local arg
    for arg in "$@"; do
      printf ' %q' "$arg"
    done
    printf '\n'
  } > "$LAUNCHER_FILE"
  chmod +x "$LAUNCHER_FILE"
}

install_cron() {
  local repo_root="$PWD"
  local args=()

  while (($#)); do
    case "$1" in
      --repo-root)
        repo_root="$2"
        shift 2
        ;;
      --help|-h)
        usage
        exit 0
        ;;
      *)
        args+=("$1")
        shift
        ;;
    esac
  done

  write_launcher "$repo_root" "${args[@]}"

  local current_crontab
  current_crontab="$(crontab -l 2>/dev/null || true)"
  current_crontab="$(printf '%s\n' "$current_crontab" | grep -F -v "$LAUNCHER_FILE" || true)"

  {
    if [[ -n "$current_crontab" ]]; then
      printf '%s\n' "$current_crontab"
    fi
    printf '@reboot %q >/dev/null 2>&1\n' "$LAUNCHER_FILE"
  } | crontab -

  echo "installed reboot launcher at $LAUNCHER_FILE"
}

run_child_once() {
  local fifo_dir fifo
  fifo_dir="$(mktemp -d "$STATE_DIR/fifo.XXXXXX")"
  fifo="$fifo_dir/output"
  mkfifo "$fifo"

  python3 scripts/conductor.py loop "$@" > "$fifo" 2>&1 &
  local child_pid=$!
  echo "$child_pid" > "$CHILD_PID_FILE"

  while IFS= read -r line || [[ -n "$line" ]]; do
    append_log "$line"
  done < "$fifo"

  local rc=0
  if ! wait "$child_pid"; then
    rc=$?
  fi

  rm -f "$fifo" "$CHILD_PID_FILE"
  rmdir "$fifo_dir"
  return "$rc"
}

run_supervisor() {
  acquire_lock
  trap 'exit 0' TERM INT HUP
  trap cleanup_supervisor EXIT
  echo "$$" > "$SUPERVISOR_PID_FILE"
  append_log "supervisor starting"

  while true; do
    if run_child_once "$@"; then
      append_log "conductor loop exited cleanly; restarting in ${RESTART_DELAY_SECONDS}s"
    else
      local rc=$?
      append_log "conductor loop exited with code ${rc}; restarting in ${RESTART_DELAY_SECONDS}s"
    fi
    sleep "$RESTART_DELAY_SECONDS"
  done
}

start_supervisor() {
  ensure_not_running
  nohup "$0" run "$@" >/dev/null 2>&1 &
  local launcher_pid=$!
  echo "started supervisor pid $launcher_pid"
}

main() {
  if (($# == 0)); then
    usage
    exit 1
  fi

  local subcommand="$1"
  shift

  case "$subcommand" in
    start)
      start_supervisor "$@"
      ;;
    run)
      run_supervisor "$@"
      ;;
    stop)
      stop_supervisor
      ;;
    status)
      status_supervisor
      ;;
    install-cron)
      install_cron "$@"
      ;;
    rotate-logs)
      local force="false"
      if [[ "${1:-}" == "--force" ]]; then
        force="true"
      fi
      rotate_logs_if_needed "$force"
      ;;
    --help|-h|help)
      usage
      ;;
    *)
      echo "unknown subcommand: $subcommand" >&2
      usage
      exit 1
      ;;
  esac
}

main "$@"
