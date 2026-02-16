#!/usr/bin/env bash
set -euo pipefail

# Ensure agent binaries are in PATH (opencode installs to ~/.opencode/bin)
export PATH="$HOME/.opencode/bin:$HOME/.local/bin:$PATH"

MAX_ITERATIONS="${MAX_ITERATIONS:-50}"
MAX_TIME_SEC="${MAX_TIME_SEC:-1800}"
ITER_TIMEOUT_SEC="${ITER_TIMEOUT_SEC:-900}"  # per-invocation timeout (15 min default)
WS="${WORKSPACE:-$HOME/workspace/repo}"
PROMPT="$WS/.dispatch-prompt.md"
AGENT_HARNESS="${AGENT_HARNESS:-claude}"  # claude | opencode
AGENT_MODEL="${AGENT_MODEL:-}"           # opencode model (e.g. moonshotai/kimi-k2.5)

[ ! -f "$PROMPT" ] && echo "[ralph] no prompt file at $PROMPT" >&2 && exit 1

# Validate harness
case "$AGENT_HARNESS" in
  claude|opencode) ;;
  *) echo "[ralph] unknown harness: $AGENT_HARNESS" >&2; exit 1 ;;
esac

echo "[ralph] harness=$AGENT_HARNESS model=${AGENT_MODEL:-default}"

start=$(date +%s)
i=0

while (( i < MAX_ITERATIONS )); do
  [ -f "$WS/TASK_COMPLETE" ] && echo "[ralph] TASK_COMPLETE found" && exit 0
  [ -f "$WS/TASK_COMPLETE.md" ] && echo "[ralph] TASK_COMPLETE.md found" && exit 0
  [ -f "$WS/BLOCKED.md" ] && echo "[ralph] BLOCKED: $(head -3 "$WS/BLOCKED.md")" >&2 && exit 2

  now=$(date +%s)
  (( now - start >= MAX_TIME_SEC )) && echo "[ralph] time limit (${MAX_TIME_SEC}s)" >&2 && exit 1

  i=$((i+1))
  echo "[ralph] iteration $i / $MAX_ITERATIONS at $(date -Iseconds)"

  case "$AGENT_HARNESS" in
    claude)
      timeout "$ITER_TIMEOUT_SEC" claude -p --dangerously-skip-permissions --verbose < "$PROMPT" 2>&1 || true
      ;;
    opencode)
      timeout "$ITER_TIMEOUT_SEC" opencode run ${AGENT_MODEL:+--model "$AGENT_MODEL"} --dir "$WS" "$(cat "$PROMPT")" 2>&1 || true
      ;;
  esac

  sleep 2
done

echo "[ralph] max iterations ($MAX_ITERATIONS)" >&2
exit 1
