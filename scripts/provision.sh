#!/usr/bin/env bash
set -euo pipefail

# Provision a sprite on Fly.io from its definition.
#
# Creates the sprite, uploads base config + persona, configures Claude Code,
# and takes an initial checkpoint.
#
# Usage:
#   ./scripts/provision.sh <sprite-name>    # Provision single sprite
#   ./scripts/provision.sh --all            # Provision all sprites

LOG_PREFIX="provision"
source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"

usage() {
    local exit_code="${1:-1}"
    echo "Usage: $0 [--composition <path>] <sprite-name|--all>"
    echo ""
    echo "  sprite-name              Name of sprite (matches sprites/<name>.md)"
    echo "  --all                    Provision all sprites from current composition"
    echo "  --composition <path>     Use specific composition YAML (default: compositions/v1.yaml)"
    echo ""
    echo "Environment:"
    echo "  SPRITE_CLI    Path to sprite CLI (default: sprite)"
    echo "  FLY_ORG       Fly.io organization (default: misty-step)"
    echo "  SPRITE_GITHUB_DEFAULT_USER   Shared sprite GitHub username (default: misty-step-sprites)"
    echo "  SPRITE_GITHUB_DEFAULT_EMAIL  Shared sprite GitHub email (default: <user>@users.noreply.github.com)"
    echo "  SPRITE_GITHUB_DEFAULT_TOKEN  Shared sprite GitHub token"
    echo "  SPRITE_GITHUB_USER_<SPRITE>  Per-sprite GitHub username override"
    echo "  SPRITE_GITHUB_EMAIL_<SPRITE> Per-sprite GitHub email override"
    echo "  SPRITE_GITHUB_TOKEN_<SPRITE> Per-sprite GitHub token override"
    echo "  GITHUB_TOKEN                 Legacy token fallback (if default/per-sprite token unset)"
    echo ""
    echo "Examples:"
    echo "  $0 bramble"
    echo "  $0 --all"
    echo "  $0 --composition compositions/v2.yaml --all"
    exit "$exit_code"
}

provision_sprite() {
    local name="$1"
    local definition="$SPRITES_DIR/${name}.md"

    validate_sprite_name "$name"

    if [[ ! -f "$definition" ]]; then
        err "No sprite definition found at $definition"
        exit 1
    fi

    log "=== Provisioning sprite: $name ==="

    # Step 1: Create the sprite (if it doesn't already exist)
    if sprite_exists "$name"; then
        log "Sprite '$name' already exists, skipping creation"
    else
        log "Creating sprite '$name'..."
        "$SPRITE_CLI" create "$name" -o "$ORG" --skip-console
        log "Sprite '$name' created"
    fi

    # Step 2: Create workspace directory
    log "Setting up workspace..."
    "$SPRITE_CLI" exec -o "$ORG" -s "$name" -- mkdir -p "$REMOTE_HOME/workspace"

    # Step 3: Upload base config (CLAUDE.md, hooks, skills, commands, settings)
    log "Uploading base config..."
    push_config "$name"

    # Step 4: Upload sprite persona definition
    log "Uploading persona: $name.md..."
    upload_file "$name" "$definition" "$REMOTE_HOME/workspace/PERSONA.md"

    # Step 5: Create initial MEMORY.md
    log "Creating initial MEMORY.md..."
    local composition_label
    composition_label="$(basename "$COMPOSITION" .yaml)"
    local timestamp
    timestamp="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
    "$SPRITE_CLI" exec -o "$ORG" -s "$name" -- bash -c \
        'cat > '"$REMOTE_HOME"'/workspace/MEMORY.md << MEMEOF
# MEMORY.md — $1

Sprite: $1
Provisioned: $2
Composition: $3

## Learnings

_No observations yet. Update after completing work._
MEMEOF' _ "$name" "$timestamp" "$composition_label"

    # Step 6: Set up git config + credentials (sprites cannot push without this)
    local gh_auth
    gh_auth="$(resolve_sprite_github_auth "$name")" || exit 1
    local gh_user gh_email gh_token
    IFS=$'\t' read -r gh_user gh_email gh_token <<< "$gh_auth"

    log "Configuring git identity + credentials (account: $gh_user)..."
    "$SPRITE_CLI" exec -o "$ORG" -s "$name" -- bash -c \
        'git config --global user.name "$1 ($2 sprite)" && \
         git config --global user.email "$3" && \
         git config --global credential.helper store && \
         printf "https://%s:%s@github.com\n" "$4" "$5" > /home/sprite/.git-credentials && \
         echo "Git credentials configured for $4"' _ "$name" "$gh_user" "$gh_email" "$gh_user" "$gh_token"

    # Verify git auth works (define errors out of existence — fail here, not at push time)
    log "Verifying git push access..."
    local push_test
    push_test=$("$SPRITE_CLI" exec -o "$ORG" -s "$name" -- bash -c \
        'cd /tmp && rm -rf _git_test && mkdir _git_test && cd _git_test && \
         git init -q && git remote add origin https://github.com/misty-step/cerberus.git && \
         git ls-remote origin HEAD >/dev/null 2>&1 && echo GIT_AUTH_OK || echo GIT_AUTH_FAIL' 2>&1)
    if [[ "$push_test" != *"GIT_AUTH_OK"* ]]; then
        err "Git auth verification FAILED on sprite '$name'."
        err "The sprite will not be able to push code. Fix credentials before dispatching."
        exit 1
    fi
    log "Git auth verified ✅"

    # Step 7: Bootstrap sprite environment (tools, dirs, sprite-agent)
    local bootstrap_script="$ROOT_DIR/scripts/sprite-bootstrap.sh"
    local agent_script="$ROOT_DIR/scripts/sprite-agent.sh"
    if [[ ! -f "$bootstrap_script" || ! -f "$agent_script" ]]; then
        err "Missing bootstrap assets. Expected:"
        err "  $bootstrap_script"
        err "  $agent_script"
        exit 1
    fi

    log "Running sprite bootstrap..."
    upload_file "$name" "$bootstrap_script" "/tmp/sprite-bootstrap.sh"
    upload_file "$name" "$agent_script" "/tmp/sprite-agent.sh"
    "$SPRITE_CLI" exec -o "$ORG" -s "$name" -- bash /tmp/sprite-bootstrap.sh --agent-source /tmp/sprite-agent.sh

    # Step 8: Create initial checkpoint
    log "Creating initial checkpoint..."
    "$SPRITE_CLI" checkpoint create -o "$ORG" -s "$name" 2>&1 || log "Checkpoint creation skipped (may already exist)"

    log "=== Done: $name ==="
    log ""
    log "Verify with:"
    log "  sprite exec -o $ORG -s $name -- ls -la $REMOTE_HOME/workspace/"
    log "  sprite exec -o $ORG -s $name -- cat $REMOTE_HOME/.claude/settings.json"
}

if [[ $# -eq 0 ]]; then
    usage
fi

# Parse args
PROVISION_ALL=false
TARGETS=()
while [[ $# -gt 0 ]]; do
    case "$1" in
        --composition)
            if [[ -z "${2:-}" ]]; then
                err "--composition requires a value"
                usage
            fi
            set_composition_path "$2" || exit 1
            shift 2
            ;;
        --all) PROVISION_ALL=true; shift ;;
        --help|-h) usage 0 ;;
        *) TARGETS+=("$1"); shift ;;
    esac
done

if [[ "$PROVISION_ALL" == false ]] && [[ ${#TARGETS[@]} -eq 0 ]]; then
    usage
fi
if [[ "$PROVISION_ALL" == true ]] && [[ ${#TARGETS[@]} -gt 0 ]]; then
    err "Use either --all or explicit sprite names, not both."
    usage
fi

trap lib_cleanup EXIT
prepare_settings

if [[ "$PROVISION_ALL" == true ]]; then
    log "Provisioning sprites from composition: $COMPOSITION"
    sprite_list=$(composition_sprites --strict) || exit 1
    if [[ -z "$sprite_list" ]]; then
        err "No sprites found in composition: $COMPOSITION"
        exit 1
    fi
    while IFS= read -r name; do
        provision_sprite "$name"
        echo ""
    done <<< "$sprite_list"
    log "All sprites provisioned."
else
    for name in "${TARGETS[@]}"; do
        provision_sprite "$name"
    done
fi
