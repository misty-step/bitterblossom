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

LOG_PREFIX="dispatch"
source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"

TEMPLATE_DIR="$(dirname "${BASH_SOURCE[0]}")"
WORKSPACE="$REMOTE_HOME/workspace"

# Max iterations before Ralph loop self-terminates (safety valve)
MAX_RALPH_ITERATIONS="${MAX_RALPH_ITERATIONS:-50}"

# Render ralph-prompt-template.md with task-specific values
generate_ralph_prompt() {
    local task="$1"
    local repo="${2:-}"
    local sprite="${3:-}"

    local template="$TEMPLATE_DIR/ralph-prompt-template.md"
    if [[ ! -f "$template" ]]; then
        err "Ralph prompt template not found: $template"
        exit 1
    fi

    local content
    content="$(cat "$template")"
    content="${content//\{\{TASK_DESCRIPTION\}\}/$task}"
    content="${content//\{\{REPO\}\}/${repo:-OWNER/REPO}}"
    content="${content//\{\{SPRITE_NAME\}\}/${sprite:-sprite}}"
    printf '%s' "$content"
}

setup_repo() {
    local name="$1"
    local repo="$2"

    # Validate repo format: org/repo or URL
    if [[ ! "$repo" =~ ^[a-zA-Z0-9._-]+/[a-zA-Z0-9._-]+$ ]] && [[ ! "$repo" =~ ^https:// ]]; then
        err "Invalid repo format: '$repo'. Use 'org/repo' or a full URL."
        exit 1
    fi

    log "Setting up repo: $repo"
    "$SPRITE_CLI" exec -o "$ORG" -s "$name" -- bash -c \
        "cd $WORKSPACE && \
         if [ -d '$(basename "$repo")' ]; then \
             cd '$(basename "$repo")' && git fetch origin && git pull --ff-only; \
         else \
             gh repo clone '$repo'; \
         fi"
}

upload_prompt() {
    local name="$1"
    local prompt="$2"
    local remote_path="$3"
    local tmp_prompt

    tmp_prompt="$(mktemp)"
    printf '%s' "$prompt" > "$tmp_prompt"
    "$SPRITE_CLI" exec -o "$ORG" -s "$name" \
        -file "$tmp_prompt:$remote_path" \
        -- echo "prompt uploaded"
    rm -f "$tmp_prompt"
}

# Start a Ralph loop on a sprite
start_ralph() {
    local name="$1"
    local prompt="$2"
    local repo="${3:-}"

    log "Starting Ralph loop on $name (max $MAX_RALPH_ITERATIONS iterations)..."

    # Generate and upload PROMPT.md from template
    local tmp_prompt
    tmp_prompt="$(mktemp)"
    generate_ralph_prompt "$prompt" "$repo" "$name" > "$tmp_prompt"

    "$SPRITE_CLI" exec -o "$ORG" -s "$name" \
        -file "$tmp_prompt:$WORKSPACE/PROMPT.md" \
        -- echo "PROMPT.md uploaded"
    rm -f "$tmp_prompt"

    # If repo specified, clone/pull it
    if [[ -n "$repo" ]]; then
        setup_repo "$name" "$repo"
    fi

    # Create the Ralph loop runner script directly with proper variable values
    local ralph_script
    ralph_script="$(cat <<RALPH_SCRIPT
#!/bin/bash
set -uo pipefail

WORKSPACE="$WORKSPACE"
LOG="\$WORKSPACE/ralph.log"
ITERATION=0
MAX_ITERATIONS=$MAX_RALPH_ITERATIONS

echo "[ralph] Starting loop at \$(date -u +%Y-%m-%dT%H:%M:%SZ) (max \$MAX_ITERATIONS iterations)" | tee -a "\$LOG"

while true; do
    ITERATION=\$((ITERATION + 1))
    echo "" >> "\$LOG"
    echo "[ralph] === Iteration \$ITERATION / \$MAX_ITERATIONS at \$(date -u +%Y-%m-%dT%H:%M:%SZ) ===" | tee -a "\$LOG"

    # Safety valve: max iterations
    if [ "\$ITERATION" -gt "\$MAX_ITERATIONS" ]; then
        echo "[ralph] Hit max iterations (\$MAX_ITERATIONS). Stopping." | tee -a "\$LOG"
        break
    fi

    # Check for completion signal
    if [ -f "\$WORKSPACE/TASK_COMPLETE" ]; then
        echo "[ralph] Task marked complete. Stopping." | tee -a "\$LOG"
        break
    fi

    # Check for blocked signal
    if [ -f "\$WORKSPACE/BLOCKED.md" ]; then
        echo "[ralph] Task blocked. See BLOCKED.md. Stopping." | tee -a "\$LOG"
        break
    fi

    # Run Claude Code with the prompt (piped, no pseudo-TTY needed)
    cd "\$WORKSPACE"
    cat PROMPT.md | claude -p --permission-mode bypassPermissions >> "\$LOG" 2>&1

    EXIT_CODE=\$?
    echo "[ralph] Claude exited with code \$EXIT_CODE at \$(date -u +%Y-%m-%dT%H:%M:%SZ)" | tee -a "\$LOG"

    # Heartbeat: log iteration summary for observability
    echo "[ralph] heartbeat: iteration=\$ITERATION exit=\$EXIT_CODE ts=\$(date -u +%Y-%m-%dT%H:%M:%SZ)" >> "\$LOG"

    # Brief pause between iterations
    sleep 5
done

echo "[ralph] Loop ended after \$ITERATION iterations at \$(date -u +%Y-%m-%dT%H:%M:%SZ)" | tee -a "\$LOG"
RALPH_SCRIPT
)"

    # Upload the script
    local tmp_script
    tmp_script="$(mktemp)"
    printf '%s' "$ralph_script" > "$tmp_script"
    "$SPRITE_CLI" exec -o "$ORG" -s "$name" \
        -file "$tmp_script:$WORKSPACE/ralph-loop.sh" \
        -- chmod +x "$WORKSPACE/ralph-loop.sh"
    rm -f "$tmp_script"

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
    local repo="${3:-}"
    local remote_prompt="$WORKSPACE/.dispatch-prompt.md"

    log "=== One-shot dispatch to $name ==="
    log "Prompt: ${prompt:0:200}$([ ${#prompt} -gt 200 ] && echo '...')"
    log ""

    if [[ -n "$repo" ]]; then
        setup_repo "$name" "$repo"
    fi

    upload_prompt "$name" "$prompt" "$remote_prompt"

    "$SPRITE_CLI" exec -o "$ORG" -s "$name" -- bash -c \
        "cd $WORKSPACE && \
         cat .dispatch-prompt.md | claude -p --permission-mode bypassPermissions 2>&1 | \
         grep -v '^\$'; \
         rm -f .dispatch-prompt.md"

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
    echo ""
    echo "Environment:"
    echo "  MAX_RALPH_ITERATIONS  Safety cap for Ralph loops (default: 50)"
    exit 1
fi

SPRITE_NAME="$1"
shift

if [[ "$SPRITE_NAME" == "--help" || "$SPRITE_NAME" == "-h" ]]; then
    exec "$0"  # Re-run with no args to show usage
fi

validate_sprite_name "$SPRITE_NAME"

if ! sprite_exists "$SPRITE_NAME"; then
    err "Sprite '$SPRITE_NAME' does not exist"
    exit 1
fi

# Preflight check: verify sprite is ready to work (Ousterhout: define errors out of existence)
# Skip for --stop and --status actions
if [[ "${1:-}" != "--stop" && "${1:-}" != "--status" ]]; then
    PREFLIGHT_SCRIPT="$(dirname "${BASH_SOURCE[0]}")/preflight.sh"
    if [[ -x "$PREFLIGHT_SCRIPT" ]]; then
        log "Running preflight checks..."
        if ! "$PREFLIGHT_SCRIPT" "$SPRITE_NAME"; then
            err "Preflight FAILED for '$SPRITE_NAME'. Fix issues before dispatching."
            err "Run: ./scripts/preflight.sh $SPRITE_NAME"
            exit 1
        fi
    fi
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
        --repo)
            if [[ $# -lt 2 ]]; then
                err "--repo requires a value (<org/repo>)"
                exit 1
            fi
            REPO="$2"
            shift 2
            ;;
        --file)
            if [[ $# -lt 2 ]]; then
                err "--file requires a path"
                exit 1
            fi
            PROMPT_FILE="$2"
            shift 2
            ;;
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
    dispatch_oneshot "$SPRITE_NAME" "$PROMPT" "$REPO"
fi
