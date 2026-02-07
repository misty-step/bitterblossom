#!/usr/bin/env bash
# watchdog-v2.sh — Active sprite fleet management
# Detects signals, takes action, wakes main session when human judgment needed.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
source "$SCRIPT_DIR/lib.sh" 2>/dev/null || true

SPRITE_CLI="${HOME}/.local/bin/sprite"
ACTIVITY_LOG="${HOME}/.openclaw/workspace/infra/activity-log.sh"
ALERTS=""
ACTIONS=""

log_activity() {
  [ -x "$ACTIVITY_LOG" ] && "$ACTIVITY_LOG" "$1" "sprite-watchdog" 2>/dev/null || true
}

SPRITES=$("$SPRITE_CLI" list 2>/dev/null || echo "")
[ -z "$SPRITES" ] && echo "No sprites found" && exit 0

for name in $SPRITES; do
  echo "=== $name ==="
  
  # Check if Claude is running
  CLAUDE_COUNT=$("$SPRITE_CLI" exec -s "$name" -- bash -c "ps aux | grep 'claude -p' | grep -v grep | wc -l" 2>/dev/null || echo "0")
  CLAUDE_COUNT=$(echo "$CLAUDE_COUNT" | tr -d '[:space:]')
  
  # Check signals
  HAS_COMPLETE=$("$SPRITE_CLI" exec -s "$name" -- bash -c "test -f /home/sprite/workspace/TASK_COMPLETE && echo yes || echo no" 2>/dev/null || echo "no")
  HAS_COMPLETE=$(echo "$HAS_COMPLETE" | tr -d '[:space:]')
  
  HAS_BLOCKED=$("$SPRITE_CLI" exec -s "$name" -- bash -c "test -f /home/sprite/workspace/BLOCKED.md && echo yes || echo no" 2>/dev/null || echo "no")
  HAS_BLOCKED=$(echo "$HAS_BLOCKED" | tr -d '[:space:]')
  
  # Get current task from active-agents.txt
  CURRENT_TASK=$(grep "^$name|" /tmp/active-agents.txt 2>/dev/null | head -1 | cut -d'|' -f2 || echo "unknown")
  
  if [ "$HAS_COMPLETE" = "yes" ]; then
    echo "  🏁 TASK_COMPLETE — pushing and opening PR"
    
    # Auto-push unpushed commits
    "$SPRITE_CLI" exec -s "$name" -- bash -c '
      for d in /home/sprite/workspace/*/; do
        [ -d "$d/.git" ] || continue
        cd "$d"
        BRANCH=$(git branch --show-current 2>/dev/null)
        [ -z "$BRANCH" ] && continue
        UNPUSHED=$(git log "origin/$BRANCH..HEAD" --oneline 2>/dev/null | wc -l)
        if [ "$UNPUSHED" -gt 0 ]; then
          git push origin "$BRANCH" 2>&1 || true
        fi
      done
    ' 2>/dev/null || true
    
    ALERTS="${ALERTS}COMPLETE: $name finished '$CURRENT_TASK'. Needs reassignment.\n"
    log_activity "Sprite $name completed: $CURRENT_TASK"
    
  elif [ "$HAS_BLOCKED" = "yes" ]; then
    REASON=$("$SPRITE_CLI" exec -s "$name" -- bash -c "head -5 /home/sprite/workspace/BLOCKED.md" 2>/dev/null || echo "unknown reason")
    ALERTS="${ALERTS}BLOCKED: $name on '$CURRENT_TASK': $REASON\n"
    log_activity "Sprite $name BLOCKED: $CURRENT_TASK — $REASON"
    
  elif [ "$CLAUDE_COUNT" -eq 0 ]; then
    echo "  💀 DEAD — Claude not running, no signal"
    ALERTS="${ALERTS}DEAD: $name — Claude not running, task was '$CURRENT_TASK'. Needs intervention.\n"
    log_activity "Sprite $name DEAD: Claude exited without signal on $CURRENT_TASK"
    
  else
    # Claude is running — check if making progress
    # 1. Any branch created? (signals work has started)
    BRANCH=$("$SPRITE_CLI" exec -s "$name" -- bash -c '
      for d in /home/sprite/workspace/*/; do
        [ -d "$d/.git" ] || continue
        cd "$d"
        B=$(git branch --show-current 2>/dev/null)
        [ "$B" != "master" ] && [ "$B" != "main" ] && [ -n "$B" ] && echo "$B" && exit
      done
    ' 2>/dev/null || echo "")
    BRANCH=$(echo "$BRANCH" | tr -d '[:space:]')

    # 2. Any commits in last 2h?
    COMMIT_COUNT=$("$SPRITE_CLI" exec -s "$name" -- bash -c '
      count=0
      for d in /home/sprite/workspace/*/; do
        [ -d "$d/.git" ] || continue
        cd "$d"
        c=$(git log --oneline --since="2 hours ago" 2>/dev/null | wc -l)
        count=$((count + c))
      done
      echo $count
    ' 2>/dev/null || echo "0")
    COMMIT_COUNT=$(echo "$COMMIT_COUNT" | tr -d '[:space:]')

    # 3. How long since dispatch?
    DISPATCH_TIME=$(grep "^$name|" /tmp/active-agents.txt 2>/dev/null | head -1 | cut -d'|' -f3 || echo "")
    ELAPSED_MIN=""
    if [ -n "$DISPATCH_TIME" ]; then
      DISPATCH_EPOCH=$(date -j -f "%Y-%m-%dT%H:%M" "$DISPATCH_TIME" "+%s" 2>/dev/null || echo "")
      if [ -n "$DISPATCH_EPOCH" ]; then
        NOW_EPOCH=$(date "+%s")
        ELAPSED_MIN=$(( (NOW_EPOCH - DISPATCH_EPOCH) / 60 ))
      fi
    fi

    # Decision logic:
    # - Claude running + has branch or commits = definitely active
    # - Claude running + no branch + no commits + <60min = still ramping (reading code)
    # - Claude running + no branch + no commits + >120min = potentially stuck
    if [ -n "$BRANCH" ] || [ "$COMMIT_COUNT" -gt 0 ]; then
      echo "  🟢 Active (claude running, branch: ${BRANCH:-master}, commits: $COMMIT_COUNT)"
    elif [ -n "$ELAPSED_MIN" ] && [ "$ELAPSED_MIN" -gt 120 ]; then
      echo "  🟡 Potentially stuck (claude running ${ELAPSED_MIN}min, no branch, no commits)"
      ALERTS="${ALERTS}STALE: $name running ${ELAPSED_MIN}min with no branch or commits on '$CURRENT_TASK'. May need investigation.\n"
    else
      echo "  🟢 Active (claude running, ramping up — ${ELAPSED_MIN:-?}min elapsed)"
    fi
  fi
done

# Create marker for next staleness check
touch /tmp/watchdog-marker 2>/dev/null || true

# Output alerts
if [ -n "$ALERTS" ]; then
  echo ""
  echo "=== ALERTS ==="
  echo -e "$ALERTS"
  echo "NEEDS_ATTENTION"
  exit 1
else
  echo ""
  echo "All sprites healthy."
  exit 0
fi
