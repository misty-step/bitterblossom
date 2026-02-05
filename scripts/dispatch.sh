#!/usr/bin/env bash
set -euo pipefail

# Dispatch a task to a sprite.
#
# Supports two modes:
#   1. One-shot: Run a single Claude Code prompt and return results
#   2. Ralph loop: Run a persistent PROMPT.md loop that keeps the sprite
#      working autonomously until the task is complete
#
# Ralph loops (https://ghuntley.com/ralph/):
#   while :; do cat PROMPT.md | claude -p ... ; done
#   When Claude stops (for any reason), the loop restarts with the same prompt.
#   Tune the PROMPT.md to handle failure modes ("keep going", "don't ask questions").
#
# Usage:
#   ./scripts/dispatch.sh <sprite> <prompt>                    # One-shot
#   ./scripts/dispatch.sh <sprite> --ralph <prompt>            # Ralph loop
#   ./scripts/dispatch.sh <sprite> --ralph --file prompt.md    # Ralph with file
#   ./scripts/dispatch.sh <sprite> --repo <org/repo> <prompt>  # With repo clone
#   ./scripts/dispatch.sh <sprite> --stop                      # Stop Ralph loop
#   ./scripts/dispatch.sh <sprite> --status                    # Check sprite

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(dirname "$SCRIPT_DIR")"
SPRITE_CLI="${SPRITE_CLI:-sprite}"
ORG="${FLY_ORG:-misty-step}"
REMOTE_HOME="/home/sprite"
WORKSPACE="$REMOTE_HOME/workspace"

log() { echo "[bitterblossom:dispatch] $*"; }
err() { echo "[bitterblossom:dispatch] ERROR: $*" >&2; }

sprite_exists() {
    "$SPRITE_CLI" list -o "$ORG" 2>/dev/null | grep -qx "$1"
}

# Generate a Ralph loop PROMPT.md with task-specific content
generate_ralph_prompt() {
    local task="$1"
    local repo="${2:-}"

    cat << RALPH_EOF
# Task

$task

# Instructions

You are working autonomously. Do NOT stop to ask clarifying questions.
If something is ambiguous, make your best judgment call and document the decision.

## Workflow
1. Read MEMORY.md and CLAUDE.md for context from previous iterations
2. Assess current state: what's done, what's left, what's broken
3. Work on the highest-priority remaining item
4. Run tests after every meaningful change
5. Commit working changes with descriptive messages
6. Update MEMORY.md with what you learned and what's left
7. If the task is COMPLETE, create a file called TASK_COMPLETE with a summary

## When you finish or get stuck
- If task is complete: write TASK_COMPLETE file, commit, push
- If genuinely blocked (missing credentials, permission error, etc.):
  write BLOCKED.md explaining exactly what you need, then stop
- Otherwise: KEEP WORKING. Don't stop for cosmetic concerns or hypothetical questions.

## Git workflow
- Work on a feature branch (never main/master)
- Commit frequently with conventional commit messages
- Push to origin when you have working changes
- Open a PR when the feature is ready
RALPH_EOF
}

# Start a Ralph loop on a sprite
start_ralph() {
    local name="$1"
    local prompt="$2"
    local repo="${3:-}"

    log "Starting Ralph loop on $name..."

    # Upload the PROMPT.md
    local tmp_prompt
    tmp_prompt="$(mktemp)"
    generate_ralph_prompt "$prompt" "$repo" > "$tmp_prompt"

    "$SPRITE_CLI" exec -o "$ORG" -s "$name" \
        -file "$tmp_prompt:$WORKSPACE/PROMPT.md" \
        -- echo "PROMPT.md uploaded"
    rm -f "$tmp_prompt"

    # If repo specified, clone/pull it
    if [[ -n "$repo" ]]; then
        log "Setting up repo: $repo"
        "$SPRITE_CLI" exec -o "$ORG" -s "$name" -- bash -c \
            "cd $WORKSPACE && \
             if [ -d '$(basename "$repo")' ]; then \
                 cd '$(basename "$repo")' && git fetch && git pull --rebase; \
             else \
                 gh repo clone '$repo'; \
             fi"
    fi

    # Create the Ralph loop runner script on the sprite
    "$SPRITE_CLI" exec -o "$ORG" -s "$name" -- bash -c "cat > $WORKSPACE/ralph-loop.sh << 'SCRIPT_EOF'
#!/bin/bash
set -uo pipefail

WORKSPACE=\"$WORKSPACE\"
LOG=\"\$WORKSPACE/ralph.log\"
ITERATION=0

echo \"[ralph] Starting loop at \$(date -u +%Y-%m-%dT%H:%M:%SZ)\" | tee -a \"\$LOG\"

while true; do
    ITERATION=\$((ITERATION + 1))
    echo \"\" >> \"\$LOG\"
    echo \"[ralph] === Iteration \$ITERATION at \$(date -u +%Y-%m-%dT%H:%M:%SZ) ===\" | tee -a \"\$LOG\"

    # Check for completion signal
    if [ -f \"\$WORKSPACE/TASK_COMPLETE\" ]; then
        echo \"[ralph] Task marked complete. Stopping.\" | tee -a \"\$LOG\"
        break
    fi

    # Check for blocked signal
    if [ -f \"\$WORKSPACE/BLOCKED.md\" ]; then
        echo \"[ralph] Task blocked. See BLOCKED.md. Stopping.\" | tee -a \"\$LOG\"
        break
    fi

    # Run Claude Code with the prompt
    cd \"\$WORKSPACE\"
    script -q -c \"claude -p --permission-mode bypassPermissions \\\"\$(cat PROMPT.md)\\\"\" /dev/null 2>&1 | \\
        sed 's/\\x1b\\[[0-9;?]*[a-zA-Z]//g' | \\
        sed 's/\\x1b\\][0-9;]*[^\\x07]*\\x07//g' | \\
        tr -d '\\r' >> \"\$LOG\" 2>&1

    EXIT_CODE=\$?
    echo \"[ralph] Claude exited with code \$EXIT_CODE\" | tee -a \"\$LOG\"

    # Brief pause between iterations
    sleep 5
done

echo \"[ralph] Loop ended after \$ITERATION iterations at \$(date -u +%Y-%m-%dT%H:%M:%SZ)\" | tee -a \"\$LOG\"
SCRIPT_EOF
chmod +x $WORKSPACE/ralph-loop.sh"

    # Replace the hardcoded WORKSPACE path
    "$SPRITE_CLI" exec -o "$ORG" -s "$name" -- \
        sed -i "s|\\\$WORKSPACE|$WORKSPACE|g" "$WORKSPACE/ralph-loop.sh"

    # Start the loop in the background via nohup
    log "Launching Ralph loop..."
    "$SPRITE_CLI" exec -o "$ORG" -s "$name" -- bash -c \
        "cd $WORKSPACE && nohup bash ralph-loop.sh > /dev/null 2>&1 & echo \$! > ralph.pid && echo \"PID: \$(cat ralph.pid)\""

    log "Ralph loop started on $name"
    log ""
    log "Monitor with:"
    log "  $0 $name --status"
    log "  sprite exec -o $ORG -s $name -- tail -50 $WORKSPACE/ralph.log"
    log ""
    log "Stop with:"
    log "  $0 $name --stop"
}

# Stop a Ralph loop
stop_ralph() {
    local name="$1"
    log "Stopping Ralph loop on $name..."

    # Kill the loop process
    "$SPRITE_CLI" exec -o "$ORG" -s "$name" -- bash -c \
        "if [ -f $WORKSPACE/ralph.pid ]; then \
             PID=\$(cat $WORKSPACE/ralph.pid); \
             kill \$PID 2>/dev/null || true; \
             pkill -f ralph-loop.sh 2>/dev/null || true; \
             pkill -f 'claude -p' 2>/dev/null || true; \
             rm -f $WORKSPACE/ralph.pid; \
             echo 'Ralph loop stopped'; \
         else \
             echo 'No Ralph loop running (no PID file)'; \
         fi"

    log "Done"
}

# Check sprite status
check_status() {
    local name="$1"

    echo "=== Sprite: $name ==="
    echo ""

    # Check if Ralph loop is running
    local ralph_status
    ralph_status=$("$SPRITE_CLI" exec -o "$ORG" -s "$name" -- bash -c \
        "if [ -f $WORKSPACE/ralph.pid ] && kill -0 \$(cat $WORKSPACE/ralph.pid) 2>/dev/null; then \
             echo 'RUNNING (PID '\$(cat $WORKSPACE/ralph.pid)')'; \
         else \
             echo 'NOT RUNNING'; \
         fi" 2>&1)
    echo "Ralph loop: $ralph_status"

    # Check for completion/blocked signals
    "$SPRITE_CLI" exec -o "$ORG" -s "$name" -- bash -c \
        "[ -f $WORKSPACE/TASK_COMPLETE ] && echo 'STATUS: TASK COMPLETE' && cat $WORKSPACE/TASK_COMPLETE; \
         [ -f $WORKSPACE/BLOCKED.md ] && echo 'STATUS: BLOCKED' && cat $WORKSPACE/BLOCKED.md; \
         [ ! -f $WORKSPACE/TASK_COMPLETE ] && [ ! -f $WORKSPACE/BLOCKED.md ] && echo 'STATUS: Working'" 2>/dev/null || true

    echo ""

    # Show recent log
    echo "Recent log:"
    "$SPRITE_CLI" exec -o "$ORG" -s "$name" -- tail -20 "$WORKSPACE/ralph.log" 2>/dev/null || echo "  (no log)"

    echo ""

    # Show MEMORY.md summary
    echo "MEMORY.md (last 10 lines):"
    "$SPRITE_CLI" exec -o "$ORG" -s "$name" -- tail -10 "$WORKSPACE/MEMORY.md" 2>/dev/null || echo "  (no MEMORY.md)"
}

# One-shot dispatch
dispatch_oneshot() {
    local name="$1"
    local prompt="$2"

    log "=== One-shot dispatch to $name ==="
    log "Prompt: ${prompt:0:200}$([ ${#prompt} -gt 200 ] && echo '...')"
    log ""

    "$SPRITE_CLI" exec -o "$ORG" -s "$name" -- bash -c \
        "cd $WORKSPACE && \
         script -q -c 'claude -p --permission-mode bypassPermissions \"$(echo "$prompt" | sed "s/\"/\\\\\"/g")\"' /dev/null 2>&1 | \
         sed 's/\x1b\[[0-9;?]*[a-zA-Z]//g' | \
         sed 's/\x1b\][0-9;]*[^\x07]*\x07//g' | \
         tr -d '\r' | \
         grep -v '^\$'"

    log ""
    log "=== Done: $name ==="
}

# --- Main ---

if [[ $# -lt 1 ]]; then
    echo "Usage: $0 <sprite> [options] <prompt>"
    echo "       $0 <sprite> --ralph [--repo org/repo] [--file prompt.md] <prompt>"
    echo "       $0 <sprite> --stop"
    echo "       $0 <sprite> --status"
    echo ""
    echo "Options:"
    echo "  --ralph          Start a Ralph loop (persistent autonomous work)"
    echo "  --repo <org/repo> Clone/pull a repo before running"
    echo "  --file <path>    Read prompt from a file"
    echo "  --stop           Stop a running Ralph loop"
    echo "  --status         Check sprite status and recent logs"
    exit 1
fi

SPRITE_NAME="$1"
shift

if [[ "$SPRITE_NAME" == "--help" || "$SPRITE_NAME" == "-h" ]]; then
    exec "$0"  # Re-run with no args to show usage
fi

if ! sprite_exists "$SPRITE_NAME"; then
    err "Sprite '$SPRITE_NAME' does not exist"
    exit 1
fi

# Parse remaining args
RALPH=false
REPO=""
PROMPT_FILE=""
PROMPT=""
ACTION=""

while [[ $# -gt 0 ]]; do
    case "$1" in
        --ralph) RALPH=true; shift ;;
        --repo) REPO="$2"; shift 2 ;;
        --file) PROMPT_FILE="$2"; shift 2 ;;
        --stop) ACTION="stop"; shift ;;
        --status) ACTION="status"; shift ;;
        *) PROMPT="$*"; break ;;
    esac
done

# Handle actions
case "$ACTION" in
    stop) stop_ralph "$SPRITE_NAME"; exit 0 ;;
    status) check_status "$SPRITE_NAME"; exit 0 ;;
esac

# Read prompt from file if specified
if [[ -n "$PROMPT_FILE" ]]; then
    if [[ ! -f "$PROMPT_FILE" ]]; then
        err "Prompt file not found: $PROMPT_FILE"
        exit 1
    fi
    PROMPT="$(cat "$PROMPT_FILE")"
fi

if [[ -z "$PROMPT" ]]; then
    err "No prompt provided"
    exit 1
fi

if [[ "$RALPH" == true ]]; then
    start_ralph "$SPRITE_NAME" "$PROMPT" "$REPO"
else
    dispatch_oneshot "$SPRITE_NAME" "$PROMPT"
fi
