#!/usr/bin/env bash
set -euo pipefail

# Dispatch a task to a sprite.
#
# Sends a prompt to a sprite's Claude Code instance, captures output.
# The sprite works autonomously and returns results.
#
# Usage:
#   ./scripts/dispatch.sh <sprite-name> <prompt>
#   ./scripts/dispatch.sh <sprite-name> --file <prompt-file>
#   ./scripts/dispatch.sh <sprite-name> --repo <org/repo> <prompt>
#   ./scripts/dispatch.sh <sprite-name> --continue           # Resume last session

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(dirname "$SCRIPT_DIR")"
SPRITE_CLI="${SPRITE_CLI:-sprite}"
ORG="${FLY_ORG:-misty-step}"
REMOTE_HOME="/home/sprite"

log() { echo "[bitterblossom:dispatch] $*"; }
err() { echo "[bitterblossom:dispatch] ERROR: $*" >&2; }

sprite_exists() {
    local name="$1"
    "$SPRITE_CLI" list -o "$ORG" 2>/dev/null | grep -qx "$name"
}

dispatch() {
    local name="$1"
    shift

    local prompt=""
    local repo=""
    local prompt_file=""
    local continue_session=false

    # Parse remaining args
    while [[ $# -gt 0 ]]; do
        case "$1" in
            --repo)
                repo="$2"
                shift 2
                ;;
            --file)
                prompt_file="$2"
                shift 2
                ;;
            --continue)
                continue_session=true
                shift
                ;;
            *)
                prompt="$*"
                break
                ;;
        esac
    done

    if [[ "$continue_session" == false && -z "$prompt" && -z "$prompt_file" ]]; then
        err "No prompt provided"
        echo "Usage: $0 <sprite-name> <prompt>"
        exit 1
    fi

    if ! sprite_exists "$name"; then
        err "Sprite '$name' does not exist. Run provision.sh first."
        exit 1
    fi

    # Read prompt from file if specified
    if [[ -n "$prompt_file" ]]; then
        if [[ ! -f "$prompt_file" ]]; then
            err "Prompt file not found: $prompt_file"
            exit 1
        fi
        prompt="$(cat "$prompt_file")"
    fi

    # Clone repo if specified
    if [[ -n "$repo" ]]; then
        log "Setting up repo: $repo"
        "$SPRITE_CLI" exec -o "$ORG" -s "$name" -- bash -c \
            "cd $REMOTE_HOME/workspace && \
             if [ -d '$(basename "$repo")' ]; then \
                 cd '$(basename "$repo")' && git pull; \
             else \
                 gh repo clone '$repo'; \
             fi"
        log "Repo ready"
    fi

    log "=== Dispatching to $name ==="
    log "Prompt: ${prompt:0:200}$([ ${#prompt} -gt 200 ] && echo '...')"
    log ""

    if [[ "$continue_session" == true ]]; then
        # Continue last session
        "$SPRITE_CLI" exec -o "$ORG" -s "$name" -tty -- bash -c \
            "cd $REMOTE_HOME/workspace && claude --continue"
    else
        # Run Claude Code with the prompt
        # Use script for pseudo-TTY, clean escape sequences
        "$SPRITE_CLI" exec -o "$ORG" -s "$name" -- bash -c \
            "cd $REMOTE_HOME/workspace && \
             script -q -c 'claude -p --permission-mode bypassPermissions \"$(echo "$prompt" | sed "s/\"/\\\\\"/g")\"' /dev/null 2>&1 | \
             sed 's/\x1b\[[0-9;?]*[a-zA-Z]//g' | \
             sed 's/\x1b\][0-9;]*[^\x07]*\x07//g' | \
             tr -d '\r' | \
             grep -v '^\$'"
    fi

    log ""
    log "=== Dispatch complete: $name ==="
}

if [[ $# -lt 1 ]]; then
    echo "Usage: $0 <sprite-name> [--repo org/repo] [--file prompt.txt] <prompt>"
    echo "       $0 <sprite-name> --continue"
    exit 1
fi

if [[ "$1" == "--help" || "$1" == "-h" ]]; then
    echo "Usage: $0 <sprite-name> [options] <prompt>"
    echo ""
    echo "Options:"
    echo "  --repo <org/repo>    Clone/pull a repo before running"
    echo "  --file <path>        Read prompt from a file"
    echo "  --continue           Resume the last Claude Code session"
    echo ""
    echo "Environment:"
    echo "  SPRITE_CLI    Path to sprite CLI (default: sprite)"
    echo "  FLY_ORG       Fly.io organization (default: misty-step)"
    exit 0
fi

dispatch "$@"
