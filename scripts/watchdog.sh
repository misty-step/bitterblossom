#!/usr/bin/env bash
# sprite-watchdog.sh — Monitor sprite fleet, auto-redispatch dead agents
# 
# Checks each sprite for running Claude Code processes.
# If a sprite has an assigned task but no running agent, redispatches it.
# Logs all actions. Designed to run every 15 min via cron.
#
# Usage: ./sprite-watchdog.sh [--dry-run]

set -euo pipefail

SPRITE_CLI="${SPRITE_CLI:-$HOME/.local/bin/sprite}"
ACTIVE_AGENTS="/tmp/active-agents.txt"
LOGFILE="$HOME/.openclaw/logs/sprite-watchdog.log"
DRY_RUN="${1:-}"

mkdir -p "$(dirname "$LOGFILE")"

log() {
  echo "[$(date '+%Y-%m-%d %H:%M:%S')] $1" | tee -a "$LOGFILE"
}

# Get list of sprites
SPRITES=$($SPRITE_CLI list 2>/dev/null || echo "")
if [ -z "$SPRITES" ]; then
  log "ERROR: Could not list sprites (sprite CLI failed)"
  exit 1
fi

TOTAL=0
ALIVE=0
DEAD=0
RECOVERED=0

for sprite in $SPRITES; do
  TOTAL=$((TOTAL + 1))
  
  # Check for running Claude process
  CLAUDE_COUNT=$($SPRITE_CLI exec -s "$sprite" -- bash -c \
    "ps aux | grep -E 'claude.*-p' | grep -v grep | wc -l" 2>/dev/null || echo "0")
  CLAUDE_COUNT=$(echo "$CLAUDE_COUNT" | tr -d '[:space:]')
  
  if [ "$CLAUDE_COUNT" -gt 0 ]; then
    ALIVE=$((ALIVE + 1))
    continue
  fi
  
  # Sprite has no Claude process. Check if it should have one.
  # Look for its task in active-agents.txt
  TASK_LINE=""
  if [ -f "$ACTIVE_AGENTS" ]; then
    TASK_LINE=$(grep "^${sprite}|" "$ACTIVE_AGENTS" 2>/dev/null || echo "")
  fi
  
  if [ -z "$TASK_LINE" ]; then
    # No assigned task — sprite is idle, that's fine
    log "INFO: $sprite — idle (no Claude process, no assigned task)"
    continue
  fi
  
  # Sprite has a task but no running agent — it's dead
  DEAD=$((DEAD + 1))
  TASK=$(echo "$TASK_LINE" | cut -d'|' -f2)
  REPO=$(echo "$TASK_LINE" | cut -d'|' -f4)
  
  log "DEAD: $sprite — task: $TASK, repo: $REPO"
  
  # Check for TASK_COMPLETE or BLOCKED signals
  COMPLETED=$($SPRITE_CLI exec -s "$sprite" -- bash -c \
    "ls /home/sprite/workspace/TASK_COMPLETE 2>/dev/null && echo 'yes' || echo 'no'" 2>/dev/null || echo "no")
  COMPLETED=$(echo "$COMPLETED" | tail -1 | tr -d '[:space:]')
  
  BLOCKED=$($SPRITE_CLI exec -s "$sprite" -- bash -c \
    "ls /home/sprite/workspace/BLOCKED.md 2>/dev/null && echo 'yes' || echo 'no'" 2>/dev/null || echo "no")
  BLOCKED=$(echo "$BLOCKED" | tail -1 | tr -d '[:space:]')
  
  if [ "$COMPLETED" = "yes" ]; then
    log "COMPLETED: $sprite finished its task. Harvesting learnings."
    # Harvest learnings if they exist
    LEARNINGS=$($SPRITE_CLI exec -s "$sprite" -- bash -c \
      "cat /home/sprite/workspace/*/LEARNINGS.md 2>/dev/null || echo ''" 2>/dev/null || echo "")
    if [ -n "$LEARNINGS" ]; then
      LEARNINGS_DIR="$(dirname "$(dirname "${BASH_SOURCE[0]}")")/observations/learnings"
      mkdir -p "$LEARNINGS_DIR"
      echo "# Learnings from $sprite ($(date +%Y-%m-%d))" > "$LEARNINGS_DIR/$(date +%Y-%m-%d)-${sprite}.md"
      echo "" >> "$LEARNINGS_DIR/$(date +%Y-%m-%d)-${sprite}.md"
      echo "$LEARNINGS" >> "$LEARNINGS_DIR/$(date +%Y-%m-%d)-${sprite}.md"
      log "HARVESTED: Learnings saved from $sprite"
    fi
    continue
  fi
  
  if [ "$BLOCKED" = "yes" ]; then
    BLOCK_REASON=$($SPRITE_CLI exec -s "$sprite" -- bash -c \
      "cat /home/sprite/workspace/BLOCKED.md 2>/dev/null" 2>/dev/null || echo "unknown reason")
    log "BLOCKED: $sprite is blocked — $BLOCK_REASON"
    continue
  fi
  
  # Dead and not completed/blocked — needs redispatch
  if [ "$DRY_RUN" = "--dry-run" ]; then
    log "DRY-RUN: Would redispatch $sprite"
    continue
  fi
  
  log "REDISPATCH: $sprite — restarting Claude Code"
  
  # Check if PROMPT.md exists on the sprite
  HAS_PROMPT=$($SPRITE_CLI exec -s "$sprite" -- bash -c \
    "test -f /home/sprite/workspace/PROMPT.md && echo 'yes' || echo 'no'" 2>/dev/null || echo "no")
  HAS_PROMPT=$(echo "$HAS_PROMPT" | tail -1 | tr -d '[:space:]')
  
  if [ "$HAS_PROMPT" = "yes" ]; then
    # Find the repo directory
    REPO_DIR=$($SPRITE_CLI exec -s "$sprite" -- bash -c \
      "basename \$(ls -d /home/sprite/workspace/*/ 2>/dev/null | grep -v -E '^\.' | head -1) 2>/dev/null || echo 'workspace'" 2>/dev/null || echo "workspace")
    REPO_DIR=$(echo "$REPO_DIR" | tail -1 | tr -d '[:space:]')
    
    # Redispatch using existing PROMPT.md
    $SPRITE_CLI exec -s "$sprite" -- bash -c \
      "cd /home/sprite/workspace/${REPO_DIR} 2>/dev/null || cd /home/sprite/workspace; \
       nohup bash -c 'cat /home/sprite/workspace/PROMPT.md | claude -p --permission-mode bypassPermissions --verbose --output-format stream-json' \
       > /home/sprite/workspace/watchdog-recovery-\$(date +%s).log 2>&1 &" 2>/dev/null || true
    
    RECOVERED=$((RECOVERED + 1))
    log "RECOVERED: $sprite redispatched successfully"
  else
    log "NO-PROMPT: $sprite has no PROMPT.md — needs manual dispatch"
  fi
done

# Summary
log "SUMMARY: $TOTAL sprites | $ALIVE alive | $DEAD dead | $RECOVERED recovered"

# If any sprites died and were recovered, send a notification
if [ $RECOVERED -gt 0 ]; then
  # Wake OpenClaw to report
  openclaw gateway wake --text "Sprite watchdog recovered $RECOVERED dead sprite(s): check logs at $LOGFILE" --mode now 2>/dev/null || true
fi
