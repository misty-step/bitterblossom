#!/usr/bin/env bash
# health-check.sh — Deep sprite health check with progress detection
#
# Goes beyond "is claude running" to check:
# - Is there ACTUAL git activity (commits, file changes)?
# - Is the workspace growing or stale?
# - Is the claude process making API calls or hung?
# - Are there error signals?
#
# Designed to catch stalled sprites FAST.
#
# Usage:
#   ./scripts/health-check.sh              # Check all sprites
#   ./scripts/health-check.sh <sprite>     # Check one sprite
#   ./scripts/health-check.sh --json       # JSON output for Overmind

set -euo pipefail

SPRITE_CLI="${SPRITE_CLI:-$HOME/.local/bin/sprite}"
JSON_MODE=false
STALE_THRESHOLD_MIN="${STALE_THRESHOLD_MIN:-30}"  # Minutes without progress = stale

if [[ "${1:-}" == "--json" ]]; then
    JSON_MODE=true
    shift
fi

SPRITE_LIST="${1:-$($SPRITE_CLI list 2>/dev/null || echo "")}"
if [[ -z "$SPRITE_LIST" ]]; then
    echo "ERROR: No sprites found" >&2
    exit 1
fi

RESULTS=()

check_sprite() {
    local name="$1"
    local status="unknown"
    local claude_running=false
    local has_git_changes=false
    local last_file_change=""
    local commit_count=0
    local stale=false
    local signals=""
    local current_task=""
    local error=""

    # Get current task from tracker
    current_task=$(grep "^${name}|" /tmp/active-agents.txt 2>/dev/null | head -1 | cut -d'|' -f2 || echo "")

    # 1. Is claude running?
    local procs
    procs=$($SPRITE_CLI exec -s "$name" -- bash -c "pgrep -c claude 2>/dev/null || echo 0" 2>&1 | tr -d '[:space:]')
    [[ "$procs" -gt 0 ]] && claude_running=true

    # 2. Check for completion/blocked signals
    local task_complete blocked
    task_complete=$($SPRITE_CLI exec -s "$name" -- bash -c "test -f /home/sprite/workspace/TASK_COMPLETE && echo yes || echo no" 2>&1 | tr -d '[:space:]')
    blocked=$($SPRITE_CLI exec -s "$name" -- bash -c "test -f /home/sprite/workspace/BLOCKED.md && echo yes || echo no" 2>&1 | tr -d '[:space:]')

    if [[ "$task_complete" == "yes" ]]; then
        signals="TASK_COMPLETE"
        status="completed"
    elif [[ "$blocked" == "yes" ]]; then
        signals="BLOCKED"
        status="blocked"
    fi

    # 3. Check git activity in repo directories
    local git_info
    git_info=$($SPRITE_CLI exec -s "$name" -- bash -c '
        for d in /home/sprite/workspace/*/; do
            [ -d "$d/.git" ] || continue
            cd "$d"
            REPO=$(basename "$d")
            BRANCH=$(git branch --show-current 2>/dev/null || echo "unknown")
            UNCOMMITTED=$(git diff --stat HEAD 2>/dev/null | tail -1)
            COMMITS_AHEAD=$(git log --oneline origin/master..HEAD 2>/dev/null | wc -l || echo 0)
            LAST_COMMIT_TIME=$(git log -1 --format="%ci" 2>/dev/null || echo "never")
            echo "REPO:$REPO BRANCH:$BRANCH AHEAD:$COMMITS_AHEAD UNCOMMITTED:$UNCOMMITTED LAST_COMMIT:$LAST_COMMIT_TIME"
        done
    ' 2>&1 || echo "no repos")

    if [[ "$git_info" != "no repos" && -n "$git_info" ]]; then
        commit_count=$(echo "$git_info" | grep -oP 'AHEAD:\K[0-9]+' | head -1 || echo "0")
        local uncommitted
        uncommitted=$(echo "$git_info" | grep -oP 'UNCOMMITTED:\K.*' | head -1 || echo "")
        [[ -n "$uncommitted" && "$uncommitted" != "" ]] && has_git_changes=true
    fi

    # 4. Check for recent file modifications (last 30 min)
    local recent_changes
    recent_changes=$($SPRITE_CLI exec -s "$name" -- bash -c "find /home/sprite/workspace -name '*.ts' -o -name '*.tsx' -o -name '*.js' -o -name '*.py' -o -name '*.sh' -o -name '*.md' | head -200 | xargs stat -c '%Y %n' 2>/dev/null | sort -rn | head -1" 2>&1 || echo "")
    
    if [[ -n "$recent_changes" ]]; then
        local latest_epoch
        latest_epoch=$(echo "$recent_changes" | awk '{print $1}')
        local now_epoch
        now_epoch=$(date +%s)
        if [[ -n "$latest_epoch" && "$latest_epoch" =~ ^[0-9]+$ ]]; then
            local age_min=$(( (now_epoch - latest_epoch) / 60 ))
            last_file_change="${age_min}m ago"
            if [[ $age_min -gt $STALE_THRESHOLD_MIN ]]; then
                stale=true
            fi
        fi
    fi

    # 5. Determine overall status
    if [[ "$status" == "unknown" ]]; then
        if [[ "$claude_running" == true ]]; then
            if [[ "$stale" == true ]]; then
                status="stale"  # Running but no progress
            elif [[ "$has_git_changes" == true ]] || [[ "$commit_count" -gt 0 ]]; then
                status="active"  # Running and producing output
            else
                status="running"  # Running but can't confirm progress
            fi
        else
            if [[ -n "$current_task" ]]; then
                status="dead"  # Has task but claude not running
            else
                status="idle"
            fi
        fi
    fi

    # Output
    if [[ "$JSON_MODE" == true ]]; then
        RESULTS+=("{\"name\":\"$name\",\"status\":\"$status\",\"claude_running\":$claude_running,\"has_git_changes\":$has_git_changes,\"commit_count\":$commit_count,\"stale\":$stale,\"last_file_change\":\"$last_file_change\",\"signals\":\"$signals\",\"current_task\":\"$current_task\"}")
    else
        local icon="❓"
        case "$status" in
            active) icon="🟢" ;;
            running) icon="🔵" ;;
            stale) icon="🟡" ;;
            dead) icon="🔴" ;;
            completed) icon="✅" ;;
            blocked) icon="🚫" ;;
            idle) icon="⚪" ;;
        esac

        echo "$icon $name: $status"
        [[ -n "$current_task" ]] && echo "   Task: $current_task"
        echo "   Claude: $([ "$claude_running" == true ] && echo "running ($procs)" || echo "not running")"
        echo "   Git: ${commit_count} commits ahead, changes=$([ "$has_git_changes" == true ] && echo "yes" || echo "no")"
        echo "   Files: last change $last_file_change"
        [[ -n "$signals" ]] && echo "   Signal: $signals"
        [[ "$stale" == true ]] && echo "   ⚠️  STALE: No file changes in >${STALE_THRESHOLD_MIN}min"
        echo ""
    fi
}

for sprite in $SPRITE_LIST; do
    check_sprite "$sprite"
done

if [[ "$JSON_MODE" == true ]]; then
    echo "[$(IFS=,; echo "${RESULTS[*]}")]"
fi
