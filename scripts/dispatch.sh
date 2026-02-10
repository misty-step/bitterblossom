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
    local task_id
    task_id="bb-$(date -u +%Y%m%d-%H%M%S)-${name}"

    log "Starting Ralph loop on $name via sprite-agent (task_id=$task_id, max $MAX_RALPH_ITERATIONS iterations)..."

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

    # Upload sprite-agent script as a fallback for older sprites.
    local local_agent="$TEMPLATE_DIR/sprite-agent.sh"
    if [[ -f "$local_agent" ]]; then
        log "Uploading sprite-agent fallback script..."
        "$SPRITE_CLI" exec -o "$ORG" -s "$name" \
            -file "$local_agent:$WORKSPACE/.sprite-agent.sh" \
            -- chmod +x "$WORKSPACE/.sprite-agent.sh"
    else
        log "Local sprite-agent.sh not found; relying on installed ~/.local/bin/sprite-agent"
    fi

    # Start sprite-agent in the background.
    log "Launching sprite-agent..."
    "$SPRITE_CLI" exec -o "$ORG" -s "$name" \
        -- bash -c '
            set -euo pipefail
            TASK_ID="$1"
            MAX_ITERS="$2"
            SPRITE_LABEL="$3"
            WEBHOOK_URL="$4"
            WORKSPACE_DIR="'"$WORKSPACE"'"

            mkdir -p "$WORKSPACE_DIR/logs"
            rm -f "$WORKSPACE_DIR/TASK_COMPLETE" "$WORKSPACE_DIR/BLOCKED.md"
            printf "%s\n" "$TASK_ID" > "$WORKSPACE_DIR/.current-task-id"

            if [[ -f "$WORKSPACE_DIR/agent.pid" ]] && kill -0 "$(cat "$WORKSPACE_DIR/agent.pid")" 2>/dev/null; then
                kill "$(cat "$WORKSPACE_DIR/agent.pid")" 2>/dev/null || true
                sleep 1
            fi
            if [[ -f "$WORKSPACE_DIR/ralph.pid" ]] && kill -0 "$(cat "$WORKSPACE_DIR/ralph.pid")" 2>/dev/null; then
                kill "$(cat "$WORKSPACE_DIR/ralph.pid")" 2>/dev/null || true
                sleep 1
            fi

            AGENT_BIN="$HOME/.local/bin/sprite-agent"
            if [[ ! -x "$AGENT_BIN" ]]; then
                AGENT_BIN="$WORKSPACE_DIR/.sprite-agent.sh"
            fi
            if [[ ! -x "$AGENT_BIN" ]]; then
                echo "ERROR: sprite-agent not found. Run sprite-bootstrap first." >&2
                exit 1
            fi

            cd "$WORKSPACE_DIR"
            if [[ -n "$WEBHOOK_URL" ]]; then
                nohup env \
                    SPRITE_NAME="$SPRITE_LABEL" \
                    SPRITE_WEBHOOK_URL="$WEBHOOK_URL" \
                    MAX_ITERATIONS="$MAX_ITERS" \
                    "$AGENT_BIN" >/dev/null 2>&1 &
            else
                nohup env \
                    SPRITE_NAME="$SPRITE_LABEL" \
                    MAX_ITERATIONS="$MAX_ITERS" \
                    "$AGENT_BIN" >/dev/null 2>&1 &
            fi

            PID="$!"
            echo "$PID" > "$WORKSPACE_DIR/agent.pid"
            echo "$PID" > "$WORKSPACE_DIR/ralph.pid"
            echo "PID: $PID"
        ' _ "$task_id" "$MAX_RALPH_ITERATIONS" "$name" "${SPRITE_WEBHOOK_URL:-}"

    log "Ralph loop started on $name (managed by sprite-agent)"
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
    log "Stopping Ralph loop on $name (sprite-agent + legacy processes)..."

    "$SPRITE_CLI" exec -o "$ORG" -s "$name" -- bash -c \
        "if [ -f $WORKSPACE/agent.pid ]; then \
             PID=\$(cat $WORKSPACE/agent.pid); \
             kill \$PID 2>/dev/null || true; \
         fi; \
         if [ -f $WORKSPACE/ralph.pid ]; then \
             PID=\$(cat $WORKSPACE/ralph.pid); \
             kill \$PID 2>/dev/null || true; \
         fi; \
         pkill -f sprite-agent 2>/dev/null || true; \
         pkill -f ralph-loop.sh 2>/dev/null || true; \
         pkill -f 'claude -p' 2>/dev/null || true; \
         rm -f $WORKSPACE/agent.pid $WORKSPACE/ralph.pid; \
         echo 'Ralph loop stopped'"

    log "Done"
}

# Check sprite status
check_status() {
    local name="$1"

    echo "=== Sprite: $name ==="
    echo ""

    # Check if sprite-agent / Ralph loop is running
    local ralph_status
    ralph_status=$("$SPRITE_CLI" exec -o "$ORG" -s "$name" -- bash -c \
        "if [ -f $WORKSPACE/agent.pid ] && kill -0 \$(cat $WORKSPACE/agent.pid) 2>/dev/null; then \
             echo 'RUNNING sprite-agent (PID '\$(cat $WORKSPACE/agent.pid)')'; \
         elif [ -f $WORKSPACE/ralph.pid ] && kill -0 \$(cat $WORKSPACE/ralph.pid) 2>/dev/null; then \
             echo 'RUNNING legacy loop (PID '\$(cat $WORKSPACE/ralph.pid)')'; \
         else \
             echo 'NOT RUNNING'; \
         fi" 2>&1)
    echo "Supervisor: $ralph_status"

    local current_task_id
    current_task_id=$("$SPRITE_CLI" exec -o "$ORG" -s "$name" -- bash -c \
        "cat $WORKSPACE/.current-task-id 2>/dev/null || true" 2>/dev/null | tr -d '\r\n')
    if [[ -n "$current_task_id" ]]; then
        echo "Task ID: $current_task_id"
    fi

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
         cat .dispatch-prompt.md | claude -p --permission-mode bypassPermissions --verbose --output-format stream-json 2>&1 | \
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
