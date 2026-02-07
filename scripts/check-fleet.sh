#!/usr/bin/env bash
set -euo pipefail

# check-fleet.sh — Check status of all sprites in the fleet
#
# Usage: check-fleet.sh [sprite1 sprite2 ...]
#        check-fleet.sh --all
#
# Reads STATUS.json and AGENT_PID from each sprite to determine:
# - RUNNING: agent process is alive
# - DONE: agent process exited (check log for result)
# - IDLE: no agent dispatched
#
# Output: tab-separated status per sprite

SPRITE_CLI="${SPRITE_CLI:-sprite}"

if [[ "${1:-}" == "--all" ]]; then
  SPRITES=$(${SPRITE_CLI} list 2>/dev/null | tr '\n' ' ')
else
  SPRITES="${*:-bramble fern moss thorn willow hemlock hazel}"
fi

printf "%-12s %-10s %-30s %-20s %s\n" "SPRITE" "STATUS" "TASK" "STARTED" "RUNTIME"
printf "%-12s %-10s %-30s %-20s %s\n" "------" "------" "----" "-------" "-------"

for sprite in ${SPRITES}; do
  STATUS_JSON=$(${SPRITE_CLI} exec -s "${sprite}" -- bash -c '
    if [ -f /home/sprite/workspace/STATUS.json ]; then
      cat /home/sprite/workspace/STATUS.json
    else
      echo "{}"
    fi
    
    if [ -f /home/sprite/workspace/AGENT_PID ]; then
      PID=$(cat /home/sprite/workspace/AGENT_PID)
      if kill -0 "$PID" 2>/dev/null; then
        echo "AGENT_ALIVE"
      else
        echo "AGENT_DEAD"
      fi
    else
      # Check for any claude process
      if pgrep -f "claude -p" > /dev/null 2>&1; then
        echo "AGENT_ALIVE"
      else
        echo "AGENT_DEAD"
      fi
    fi
  ' 2>/dev/null || echo -e "{}\nAGENT_DEAD")

  JSON_LINE=$(echo "${STATUS_JSON}" | head -1)
  ALIVE_LINE=$(echo "${STATUS_JSON}" | tail -1)

  REPO=$(echo "${JSON_LINE}" | grep -o '"repo":"[^"]*"' | cut -d'"' -f4 2>/dev/null || echo "-")
  ISSUE=$(echo "${JSON_LINE}" | grep -o '"issue":[0-9]*' | cut -d: -f2 2>/dev/null || echo "-")
  STARTED=$(echo "${JSON_LINE}" | grep -o '"started":"[^"]*"' | cut -d'"' -f4 2>/dev/null || echo "-")

  if [[ "${ALIVE_LINE}" == "AGENT_ALIVE" ]]; then
    STATE="RUNNING"
  elif [[ "${REPO}" != "-" ]]; then
    STATE="DONE"
  else
    STATE="IDLE"
  fi

  TASK="${REPO}#${ISSUE}"
  [[ "${TASK}" == "-#-" ]] && TASK="-"

  printf "%-12s %-10s %-30s %-20s\n" "${sprite}" "${STATE}" "${TASK}" "${STARTED}"
done
