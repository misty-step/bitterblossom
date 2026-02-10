#!/usr/bin/env bash
set -euo pipefail

# sprite-agent.sh — Ralph loop supervisor + event emitter
#
# Runs on each sprite and wraps Claude Code with:
# - Structured local events (JSONL)
# - Optional webhook POST delivery
# - Heartbeats/progress reporting
# - Periodic health checks and git auto-push
#
# Usage:
#   ./scripts/sprite-agent.sh
#   MAX_ITERATIONS=75 SPRITE_WEBHOOK_URL="https://collector/events" ./scripts/sprite-agent.sh

usage() {
    local exit_code="${1:-0}"
    cat <<'EOF'
Usage: sprite-agent.sh

Environment:
  SPRITE_NAME          Sprite identity for event payloads (default: hostname)
  SPRITE_WEBHOOK_URL   Optional webhook URL for POSTing events (default: unset)
  WORKSPACE            Workspace root (default: $HOME/workspace)
  MAX_ITERATIONS       Ralph safety cap (default: 50)
  HEARTBEAT_INTERVAL   Seconds between heartbeat events (default: 300)
  PROGRESS_INTERVAL    Seconds between progress events (default: 900)
  PUSH_INTERVAL        Seconds between git auto-push runs (default: 1800)
  HEALTH_INTERVAL      Seconds between health checks (default: 120)
  LOOP_SLEEP_SEC       Poll cadence while Claude runs (default: 5)
EOF
    exit "$exit_code"
}

if [[ "${1:-}" == "--help" || "${1:-}" == "-h" ]]; then
    usage 0
fi

SPRITE_NAME="${SPRITE_NAME:-$(hostname -s 2>/dev/null || hostname)}"
SPRITE_WEBHOOK_URL="${SPRITE_WEBHOOK_URL:-}"
WORKSPACE="${WORKSPACE:-$HOME/workspace}"
PROMPT_FILE="$WORKSPACE/PROMPT.md"
TASK_ID_FILE="$WORKSPACE/.current-task-id"
TASK_COMPLETE_FILE="$WORKSPACE/TASK_COMPLETE"
BLOCKED_FILE="$WORKSPACE/BLOCKED.md"
LOG_DIR="$WORKSPACE/logs"
EVENT_LOG="$LOG_DIR/agent.jsonl"
RALPH_LOG="$WORKSPACE/ralph.log"

MAX_ITERATIONS="${MAX_ITERATIONS:-50}"
HEARTBEAT_INTERVAL="${HEARTBEAT_INTERVAL:-300}"
PROGRESS_INTERVAL="${PROGRESS_INTERVAL:-900}"
PUSH_INTERVAL="${PUSH_INTERVAL:-1800}"
HEALTH_INTERVAL="${HEALTH_INTERVAL:-120}"
LOOP_SLEEP_SEC="${LOOP_SLEEP_SEC:-5}"

HAS_SCRIPT_PTY=false
if command -v script >/dev/null 2>&1; then
    if script -qefc "true" /dev/null >/dev/null 2>&1; then
        HAS_SCRIPT_PTY=true
    fi
fi

for v in MAX_ITERATIONS HEARTBEAT_INTERVAL PROGRESS_INTERVAL PUSH_INTERVAL HEALTH_INTERVAL LOOP_SLEEP_SEC; do
    if [[ ! "${!v}" =~ ^[0-9]+$ ]]; then
        echo "[sprite-agent] ERROR: $v must be a non-negative integer (got '${!v}')" >&2
        exit 1
    fi
done

mkdir -p "$LOG_DIR"
touch "$EVENT_LOG" "$RALPH_LOG"

started_epoch="$(date +%s)"
last_heartbeat=0
last_progress=0
last_push=0
last_health=0
iteration=0
current_runner_pid=""
shutdown_requested=false
shutdown_signal=""
terminal_event_emitted=false
health_json='{"cpu":"unknown","mem":"unknown","disk":"unknown","claude_running":false,"iteration":0}'

current_task_id() {
    if [[ -f "$TASK_ID_FILE" ]]; then
        tr -d '\r\n' < "$TASK_ID_FILE"
    else
        printf ''
    fi
}

emit_event() {
    local event_type="$1"
    local metadata="${2:-}"
    local ts
    local payload
    local task_id

    if [[ -z "$metadata" ]]; then
        metadata='{}'
    fi

    ts="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
    task_id="$(current_task_id)"
    payload="$(jq -cn \
        --arg sprite "$SPRITE_NAME" \
        --arg event "$event_type" \
        --arg timestamp "$ts" \
        --arg task_id "$task_id" \
        --argjson metadata "$metadata" \
        '{sprite:$sprite,event:$event,timestamp:$timestamp,task_id:$task_id,metadata:$metadata}')"

    printf '%s\n' "$payload" >> "$EVENT_LOG"

    if [[ -n "$SPRITE_WEBHOOK_URL" ]]; then
        (
            curl -fsS \
                --max-time 5 \
                -X POST \
                -H "Content-Type: application/json" \
                -d "$payload" \
                "$SPRITE_WEBHOOK_URL" >/dev/null 2>&1 || true
        ) &
    fi
}

collect_health_json() {
    local cpu="unknown"
    local mem="unknown"
    local disk="unknown"
    local claude_running=false

    cpu="$(top -bn1 2>/dev/null | awk '
        /^%?Cpu/ {
            for (i=1; i<=NF; i++) {
                if ($i == "id,") {
                    idle=$(i-1)
                    gsub(/,/, "", idle)
                    printf "%.0f%%\n", 100-idle
                    exit
                }
            }
        }'
    )"
    cpu="${cpu:-unknown}"

    mem="$(free -m 2>/dev/null | awk '/^Mem:/ { if ($2 > 0) printf "%.0f%%\n", ($3/$2)*100; else print "unknown" }')"
    mem="${mem:-unknown}"

    disk="$(df -P "$WORKSPACE" 2>/dev/null | awk 'NR==2 {print $5}')"
    disk="${disk:-unknown}"

    if pgrep -f "claude -p" >/dev/null 2>&1; then
        claude_running=true
    fi

    jq -cn \
        --arg cpu "$cpu" \
        --arg mem "$mem" \
        --arg disk "$disk" \
        --argjson claude_running "$claude_running" \
        --argjson iteration "$iteration" \
        '{cpu:$cpu,mem:$mem,disk:$disk,claude_running:$claude_running,iteration:$iteration}'
}

collect_git_metrics_json() {
    local repos=0
    local dirty=0
    local ahead=0
    local gitdir

    while IFS= read -r gitdir; do
        local repo
        local upstream=""
        local ahead_count="0"
        repo="${gitdir%/.git}"
        repos=$((repos + 1))

        if ! git -C "$repo" diff --quiet --ignore-submodules -- 2>/dev/null; then
            dirty=$((dirty + 1))
        fi

        upstream="$(git -C "$repo" rev-parse --abbrev-ref --symbolic-full-name '@{u}' 2>/dev/null || true)"
        if [[ -z "$upstream" ]]; then
            local branch
            branch="$(git -C "$repo" rev-parse --abbrev-ref HEAD 2>/dev/null || true)"
            if [[ -n "$branch" ]] && git -C "$repo" show-ref --verify --quiet "refs/remotes/origin/$branch"; then
                upstream="origin/$branch"
            fi
        fi

        if [[ -n "$upstream" ]]; then
            ahead_count="$(git -C "$repo" rev-list --count "${upstream}..HEAD" 2>/dev/null || echo 0)"
            if [[ "$ahead_count" =~ ^[0-9]+$ ]]; then
                ahead=$((ahead + ahead_count))
            fi
        fi
    done < <(find "$WORKSPACE" -mindepth 2 -maxdepth 3 -type d -name .git 2>/dev/null || true)

    jq -cn \
        --argjson repos "$repos" \
        --argjson dirty_repos "$dirty" \
        --argjson ahead_commits "$ahead" \
        '{repos:$repos,dirty_repos:$dirty_repos,ahead_commits:$ahead_commits}'
}

run_auto_push() {
    local pushed=0
    local failed=0
    local gitdir

    while IFS= read -r gitdir; do
        local repo
        local branch
        local upstream=""
        local ahead="0"

        repo="${gitdir%/.git}"
        branch="$(git -C "$repo" rev-parse --abbrev-ref HEAD 2>/dev/null || true)"
        [[ -z "$branch" ]] && continue

        upstream="$(git -C "$repo" rev-parse --abbrev-ref --symbolic-full-name '@{u}' 2>/dev/null || true)"
        if [[ -z "$upstream" ]] && git -C "$repo" show-ref --verify --quiet "refs/remotes/origin/$branch"; then
            upstream="origin/$branch"
        fi
        [[ -z "$upstream" ]] && continue

        ahead="$(git -C "$repo" rev-list --count "${upstream}..HEAD" 2>/dev/null || echo 0)"
        if [[ "$ahead" =~ ^[0-9]+$ ]] && (( ahead > 0 )); then
            if git -C "$repo" push origin "$branch" >> "$RALPH_LOG" 2>&1; then
                pushed=$((pushed + 1))
            else
                failed=$((failed + 1))
            fi
        fi
    done < <(find "$WORKSPACE" -mindepth 2 -maxdepth 3 -type d -name .git 2>/dev/null || true)

    printf '[agent] auto-push pushed=%d failed=%d at %s\n' \
        "$pushed" "$failed" "$(date -u +%Y-%m-%dT%H:%M:%SZ)" >> "$RALPH_LOG"
}

duration_minutes() {
    local now
    now="$(date +%s)"
    echo $(( (now - started_epoch) / 60 ))
}

emit_terminal_event() {
    local event_type="$1"
    local extra_metadata="${2:-}"
    local meta

    if [[ -z "$extra_metadata" ]]; then
        extra_metadata='{}'
    fi

    meta="$(jq -cn \
        --argjson health "$health_json" \
        --argjson extra "$extra_metadata" \
        --argjson duration_min "$(duration_minutes)" \
        --argjson iteration "$iteration" \
        '$health + $extra + {duration_min:$duration_min,iteration:$iteration}')"

    emit_event "$event_type" "$meta"
    terminal_event_emitted=true
}

check_terminal_signals() {
    if [[ -f "$TASK_COMPLETE_FILE" ]]; then
        emit_terminal_event "task_complete" '{"reason":"TASK_COMPLETE file detected"}'
        return 0
    fi

    if [[ -f "$BLOCKED_FILE" ]]; then
        local blocked_preview
        blocked_preview="$(head -n 6 "$BLOCKED_FILE" 2>/dev/null | tr '\n' ' ' | sed 's/[[:space:]]\+/ /g')"
        emit_terminal_event "task_blocked" "$(jq -cn \
            --arg summary "${blocked_preview:-blocked}" \
            '{reason:"BLOCKED.md detected",blocked_summary:$summary}')"
        return 0
    fi

    return 1
}

run_periodic_tasks() {
    local now="$1"

    if (( now - last_health >= HEALTH_INTERVAL )); then
        health_json="$(collect_health_json)"
        last_health="$now"
        printf '[agent] health %s\n' "$health_json" >> "$RALPH_LOG"
    fi

    if (( now - last_heartbeat >= HEARTBEAT_INTERVAL )); then
        emit_event "heartbeat" "$health_json"
        last_heartbeat="$now"
    fi

    if (( now - last_progress >= PROGRESS_INTERVAL )); then
        local progress_json
        progress_json="$(jq -cn \
            --argjson health "$health_json" \
            --argjson git "$(collect_git_metrics_json)" \
            '$health + $git')"
        emit_event "progress" "$progress_json"
        last_progress="$now"
    fi

    if (( now - last_push >= PUSH_INTERVAL )); then
        run_auto_push
        last_push="$now"
    fi
}

stop_runner_if_needed() {
    if [[ -n "$current_runner_pid" ]]; then
        kill "$current_runner_pid" >/dev/null 2>&1 || true
        wait "$current_runner_pid" 2>/dev/null || true
        current_runner_pid=""
    fi
}

run_claude_once() {
    local prompt_file="$1"
    local log_file="$2"

    # Prefer PTY-backed execution for near-real-time flush behavior.
    if [[ "$HAS_SCRIPT_PTY" == true ]]; then
        script -qefc "claude -p --dangerously-skip-permissions --permission-mode bypassPermissions --verbose --output-format stream-json < \"$prompt_file\"" \
            /dev/null >> "$log_file" 2>&1
        return
    fi

    claude -p --dangerously-skip-permissions --permission-mode bypassPermissions --verbose --output-format stream-json < "$prompt_file" >> "$log_file" 2>&1
}

on_signal() {
    shutdown_requested=true
    shutdown_signal="$1"
    stop_runner_if_needed
}

trap 'on_signal TERM' TERM
trap 'on_signal INT' INT

if [[ ! -f "$PROMPT_FILE" ]]; then
    health_json="$(collect_health_json)"
    emit_terminal_event "task_failed" '{"reason":"PROMPT.md missing"}'
    echo "[agent] PROMPT.md not found at $PROMPT_FILE" >&2
    exit 1
fi

health_json="$(collect_health_json)"
emit_event "heartbeat" "$health_json"
last_heartbeat="$(date +%s)"
last_health="$last_heartbeat"
last_progress="$last_heartbeat"
last_push="$last_heartbeat"
printf '[agent] started sprite=%s max_iterations=%s task_id=%s\n' \
    "$SPRITE_NAME" "$MAX_ITERATIONS" "$(current_task_id)" >> "$RALPH_LOG"

while (( iteration < MAX_ITERATIONS )); do
    if check_terminal_signals; then
        stop_runner_if_needed
        exit 0
    fi

    now_epoch="$(date +%s)"
    run_periodic_tasks "$now_epoch"

    if [[ "$shutdown_requested" == true ]]; then
        break
    fi

    iteration=$((iteration + 1))
    printf '\n[agent] iteration=%d started_at=%s\n' \
        "$iteration" "$(date -u +%Y-%m-%dT%H:%M:%SZ)" >> "$RALPH_LOG"

    (
        cd "$WORKSPACE"
        run_claude_once "$PROMPT_FILE" "$RALPH_LOG"
    ) &
    current_runner_pid="$!"

    while kill -0 "$current_runner_pid" >/dev/null 2>&1; do
        local_now="$(date +%s)"
        run_periodic_tasks "$local_now"

        if check_terminal_signals; then
            stop_runner_if_needed
            exit 0
        fi

        if [[ "$shutdown_requested" == true ]]; then
            stop_runner_if_needed
            break 2
        fi

        sleep "$LOOP_SLEEP_SEC"
    done

    set +e
    wait "$current_runner_pid"
    exit_code="$?"
    set -e
    current_runner_pid=""

    now_epoch="$(date +%s)"
    run_periodic_tasks "$now_epoch"

    printf '[agent] iteration=%d claude_exit=%s at=%s\n' \
        "$iteration" "$exit_code" "$(date -u +%Y-%m-%dT%H:%M:%SZ)" >> "$RALPH_LOG"
done

if [[ "$terminal_event_emitted" == false ]]; then
    health_json="$(collect_health_json)"
    if [[ "$shutdown_requested" == true ]]; then
        emit_terminal_event "task_failed" "{\"reason\":\"agent interrupted by ${shutdown_signal:-signal}\"}"
    else
        emit_terminal_event "task_failed" "{\"reason\":\"max iterations reached\",\"max_iterations\":$MAX_ITERATIONS}"
    fi
fi

if [[ "$shutdown_requested" == true ]]; then
    exit 130
fi
exit 1
