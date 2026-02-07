#!/usr/bin/env bash
set -euo pipefail

# fleet-status.sh — Fleet status from sprite event log (with SSH fallback)

LOG_PREFIX="fleet-status"
source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"

usage() {
    local exit_code="${1:-0}"
    cat <<EOF
Usage: $0 [--event-log <path>] [--stale-minutes <n>]

Defaults:
  event-log: /var/data/sprite-events.jsonl
  stale-minutes: 30
EOF
    exit "$exit_code"
}

EVENT_LOG="${EVENT_LOG:-/var/data/sprite-events.jsonl}"
STALE_MINUTES="${STALE_MINUTES:-30}"

while [[ $# -gt 0 ]]; do
    case "$1" in
        --event-log) EVENT_LOG="${2:-}"; shift 2 ;;
        --stale-minutes) STALE_MINUTES="${2:-}"; shift 2 ;;
        --help|-h) usage 0 ;;
        *) err "Unknown argument: $1"; usage 1 ;;
    esac
done

GREEN=$'\033[32m'
YELLOW=$'\033[33m'
RED=$'\033[31m'
RESET=$'\033[0m'

print_header() {
    printf "%-12s %-14s %-30s %-10s %s\n" "SPRITE" "LAST_EVENT" "TASK_ID" "DURATION" "STATUS"
    printf "%-12s %-14s %-30s %-10s %s\n" "------" "----------" "-------" "--------" "------"
}

print_row() {
    local sprite="$1" event="$2" task="$3" duration="$4" state="$5"
    local color="$YELLOW"
    case "$state" in
        working) color="$GREEN" ;;
        idle) color="$YELLOW" ;;
        stuck|failed) color="$RED" ;;
    esac
    printf "%-12s %-14s %-30s %-10s ${color}%s${RESET}\n" "$sprite" "$event" "$task" "$duration" "$state"
}

render_from_events() {
    python3 - "$EVENT_LOG" "$STALE_MINUTES" <<'PY'
import datetime as dt
import json
import sys

path = sys.argv[1]
stale = int(sys.argv[2])
now = dt.datetime.now(dt.timezone.utc)
latest = {}
task_first_seen = {}

def parse_ts(ts):
    if not ts:
        return None
    try:
        return dt.datetime.fromisoformat(ts.replace("Z", "+00:00"))
    except Exception:
        return None

with open(path, "r", encoding="utf-8") as f:
    for raw in f:
        raw = raw.strip()
        if not raw:
            continue
        try:
            evt = json.loads(raw)
        except Exception:
            continue
        sprite = evt.get("sprite")
        if not sprite:
            continue
        ts = parse_ts(evt.get("timestamp"))
        if ts is None:
            continue
        task_id = evt.get("task_id") or "-"
        key = (sprite, task_id)
        if task_id != "-" and key not in task_first_seen:
            task_first_seen[key] = ts
        prev = latest.get(sprite)
        if prev is None or ts >= prev["ts"]:
            latest[sprite] = {"event": evt.get("event", "unknown"), "task": task_id, "ts": ts}

for sprite in sorted(latest):
    row = latest[sprite]
    event = row["event"]
    task = row["task"]
    age_min = int((now - row["ts"]).total_seconds() // 60)
    if event in {"task_failed", "task_blocked"}:
        state = "failed"
    elif event == "task_complete":
        state = "idle"
    elif age_min > stale:
        state = "stuck"
    elif task != "-":
        state = "working"
    else:
        state = "idle"
    duration = "-"
    if task != "-":
        start = task_first_seen.get((sprite, task))
        if start is not None:
            mins = int((now - start).total_seconds() // 60)
            duration = f"{mins}m"
    print(f"{sprite}\t{event}\t{task}\t{duration}\t{state}")
PY
}

fallback_ssh_poll() {
    log "No event log found yet at $EVENT_LOG; falling back to SSH polling."
    print_header
    local sprites
    sprites=$("$SPRITE_CLI" list -o "$ORG" 2>/dev/null || echo "")
    if [[ -z "$sprites" ]]; then
        err "No sprites found via '$SPRITE_CLI list'."
        exit 1
    fi

    for sprite in $sprites; do
        local raw event state task
        raw=$("$SPRITE_CLI" exec -o "$ORG" -s "$sprite" -- bash -c \
            "if [ -f $REMOTE_HOME/workspace/BLOCKED.md ]; then echo 'task_blocked|failed'; \
             elif [ -f $REMOTE_HOME/workspace/TASK_COMPLETE ]; then echo 'task_complete|idle'; \
             elif pgrep -f 'claude -p' >/dev/null 2>&1; then echo 'heartbeat|working'; \
             else echo 'none|idle'; fi; \
             cat $REMOTE_HOME/workspace/.current-task-id 2>/dev/null || true" 2>/dev/null || echo "none|idle")
        event="$(echo "$raw" | head -1 | cut -d'|' -f1)"
        state="$(echo "$raw" | head -1 | cut -d'|' -f2)"
        task="$(echo "$raw" | sed -n '2p')"
        print_row "$sprite" "$event" "${task:--}" "-" "$state"
    done
}

if [[ ! -s "$EVENT_LOG" ]]; then
    fallback_ssh_poll
    exit 0
fi

print_header
while IFS=$'\t' read -r sprite event task duration state; do
    [[ -z "$sprite" ]] && continue
    print_row "$sprite" "$event" "$task" "$duration" "$state"
done < <(render_from_events)
