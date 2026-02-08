#!/usr/bin/env bash
set -euo pipefail

# tail-logs.sh — Real-time sprite log viewer
# Shows the last N lines of ralph.log from one or all sprites.
#
# Usage:
#   ./scripts/tail-logs.sh <sprite>          # Last 50 lines from one sprite
#   ./scripts/tail-logs.sh <sprite> -n 100   # Last 100 lines
#   ./scripts/tail-logs.sh <sprite> --follow # Follow live output
#   ./scripts/tail-logs.sh --all             # Last 20 lines from ALL sprites
#   ./scripts/tail-logs.sh --all --brief     # Last 5 lines (quick status)

source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"

LINES=50
BRIEF=false
ALL=false
FOLLOW=false
SPRITE=""

while [[ $# -gt 0 ]]; do
    case "$1" in
        --all) ALL=true; shift ;;
        --brief) BRIEF=true; LINES=5; shift ;;
        --follow|-f) FOLLOW=true; shift ;;
        -n) LINES="$2"; shift 2 ;;
        *) SPRITE="$1"; shift ;;
    esac
done

tail_sprite() {
    local name="$1"
    local n="${2:-$LINES}"
    
    echo "╔══════════════════════════════════════════════════════════╗"
    echo "║  📋 $name — last $n lines"
    echo "╚══════════════════════════════════════════════════════════╝"
    
    # Get process status
    local claude_pid
    claude_pid=$("$SPRITE_CLI" exec -o "$ORG" -s "$name" -- pgrep -la claude 2>/dev/null || echo "NOT RUNNING")
    echo "  Claude: $claude_pid"
    
    # Check signals
    "$SPRITE_CLI" exec -o "$ORG" -s "$name" -- bash -c \
        "[ -f $WORKSPACE/TASK_COMPLETE ] && echo '  🏁 TASK_COMPLETE'; \
         [ -f $WORKSPACE/BLOCKED.md ] && echo '  🚧 BLOCKED'; \
         [ -f $WORKSPACE/LEARNINGS.md ] && echo '  📚 Has LEARNINGS.md'" 2>/dev/null || true
    
    echo ""
    
    # Tail the log
    if [[ "$FOLLOW" == true ]]; then
        "$SPRITE_CLI" exec -o "$ORG" -s "$name" -- tail -n "$n" -f "$WORKSPACE/ralph.log" 2>/dev/null || echo "  (no ralph.log)"
    else
        "$SPRITE_CLI" exec -o "$ORG" -s "$name" -- tail -"$n" "$WORKSPACE/ralph.log" 2>/dev/null || echo "  (no ralph.log)"
    fi
    echo ""
}

if [[ "$ALL" == true ]]; then
    SPRITES=$("$SPRITE_CLI" list -o "$ORG" 2>/dev/null)
    for s in $SPRITES; do
        tail_sprite "$s" "$LINES"
    done
elif [[ -n "$SPRITE" ]]; then
    validate_sprite_name "$SPRITE"
    tail_sprite "$SPRITE" "$LINES"
else
    echo "Usage: $0 <sprite> [-n lines] [--follow] | --all [--brief] [--follow]"
    exit 1
fi
