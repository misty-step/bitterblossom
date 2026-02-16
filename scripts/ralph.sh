#!/usr/bin/env bash
set -euo pipefail

# Ensure agent binaries are in PATH (opencode installs to ~/.opencode/bin)
export PATH="$HOME/.opencode/bin:$HOME/.local/bin:$PATH"

MAX_ITERATIONS="${MAX_ITERATIONS:-50}"
MAX_TIME_SEC="${MAX_TIME_SEC:-1800}"
ITER_TIMEOUT_SEC="${ITER_TIMEOUT_SEC:-900}"  # per-invocation timeout (15 min default)
WS="${WORKSPACE:-$HOME/workspace/repo}"
PROMPT="$WS/.dispatch-prompt.md"
RALPH_LOG="$WS/ralph.log"
AGENT_HARNESS="${AGENT_HARNESS:-claude}"  # claude | opencode
AGENT_MODEL="${AGENT_MODEL:-}"           # opencode model (e.g. moonshotai/kimi-k2.5)

log() { printf '%s\n' "$*" | tee -a "$RALPH_LOG"; }
log_err() { printf '%s\n' "$*" | tee -a "$RALPH_LOG" >&2; }

touch "$RALPH_LOG"

[ ! -f "$PROMPT" ] && log_err "[ralph] no prompt file at $PROMPT" && exit 1

# Validate harness
case "$AGENT_HARNESS" in
  claude|opencode) ;;
  *) log_err "[ralph] unknown harness: $AGENT_HARNESS"; exit 1 ;;
esac

log "[ralph] harness=$AGENT_HARNESS model=${AGENT_MODEL:-default}"

start=$(date +%s)
i=0

while (( i < MAX_ITERATIONS )); do
  [ -f "$WS/TASK_COMPLETE" ] && log "[ralph] TASK_COMPLETE found" && exit 0
  [ -f "$WS/TASK_COMPLETE.md" ] && log "[ralph] TASK_COMPLETE.md found" && exit 0
  [ -f "$WS/BLOCKED.md" ] && log_err "[ralph] BLOCKED: $(head -3 "$WS/BLOCKED.md")" && exit 2

  now=$(date +%s)
  (( now - start >= MAX_TIME_SEC )) && log_err "[ralph] time limit (${MAX_TIME_SEC}s)" && exit 1

  i=$((i+1))
  log "[ralph] iteration $i / $MAX_ITERATIONS at $(date -Iseconds)"

  case "$AGENT_HARNESS" in
    claude)
      timeout "$ITER_TIMEOUT_SEC" \
        claude -p --dangerously-skip-permissions --permission-mode bypassPermissions --verbose --output-format stream-json \
        < "$PROMPT" 2>&1 | grep -F -v '"type":"system","subtype":"init"' | tee -a "$RALPH_LOG" || true
      ;;
    opencode)
      timeout "$ITER_TIMEOUT_SEC" \
        opencode run ${AGENT_MODEL:+--model "$AGENT_MODEL"} --dir "$WS" "$(cat "$PROMPT")" 2>&1 | tee -a "$RALPH_LOG" || true
      ;;
  esac

  sleep 2
done

log_err "[ralph] max iterations ($MAX_ITERATIONS)"
exit 1
