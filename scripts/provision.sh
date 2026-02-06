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

LOG_PREFIX="" source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"

usage() {
    echo "Usage: $0 <sprite-name|--all>"
    echo ""
    echo "  sprite-name   Name of sprite (matches sprites/<name>.md)"
    echo "  --all         Provision all sprites from current composition"
    echo ""
    echo "Environment:"
    echo "  SPRITE_CLI    Path to sprite CLI (default: sprite)"
    echo "  FLY_ORG       Fly.io organization (default: misty-step)"
    echo ""
    echo "Examples:"
    echo "  $0 bramble"
    echo "  $0 --all"
    exit 1
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
    "$SPRITE_CLI" exec -o "$ORG" -s "$name" -- bash -c \
        "cat > $REMOTE_HOME/workspace/MEMORY.md << 'MEMEOF'
# MEMORY.md — $name

Sprite: $name
Provisioned: $(date -u +%Y-%m-%dT%H:%M:%SZ)
Composition: v1 (Fae Court)

## Learnings

_No observations yet. Update after completing work._
MEMEOF"

    # Step 6: Set up git config AND credentials (CRITICAL — sprites can't push without this)
    local gh_token="${GITHUB_TOKEN:-}"
    if [[ -z "$gh_token" ]]; then
        # Try to get token from gh CLI
        gh_token="$(gh auth token 2>/dev/null || echo "")"
    fi
    if [[ -z "$gh_token" ]]; then
        err "GITHUB_TOKEN not set and gh CLI not authenticated."
        err "Sprites CANNOT push without git credentials. This is a hard requirement."
        err "Export GITHUB_TOKEN or authenticate gh CLI before provisioning."
        exit 1
    fi

    log "Configuring git identity + credentials..."
    "$SPRITE_CLI" exec -o "$ORG" -s "$name" -- bash -c \
        "git config --global user.name '$name (bitterblossom sprite)' && \
         git config --global user.email 'kaylee@mistystep.io' && \
         git config --global credential.helper store && \
         echo 'https://kaylee-mistystep:$gh_token@github.com' > /home/sprite/.git-credentials && \
         echo 'Git credentials configured for kaylee-mistystep'"

    # Verify git auth works (define errors out of existence — fail here, not at push time)
    log "Verifying git push access..."
    local push_test
    push_test=$("$SPRITE_CLI" exec -o "$ORG" -s "$name" -- bash -c \
        "cd /tmp && rm -rf _git_test && mkdir _git_test && cd _git_test && \
         git init -q && git remote add origin https://github.com/misty-step/cerberus.git && \
         git ls-remote origin HEAD >/dev/null 2>&1 && echo 'GIT_AUTH_OK' || echo 'GIT_AUTH_FAIL'" 2>&1)
    if [[ "$push_test" != *"GIT_AUTH_OK"* ]]; then
        err "Git auth verification FAILED on sprite '$name'."
        err "The sprite will not be able to push code. Fix credentials before dispatching."
        exit 1
    fi
    log "Git auth verified ✅"

    # Step 7: Create initial checkpoint
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

if [[ "$1" == "--all" ]]; then
    trap lib_cleanup EXIT
    prepare_settings
    log "Provisioning all sprites..."
    for def in "$SPRITES_DIR"/*.md; do
        name="$(basename "$def" .md)"
        provision_sprite "$name"
        echo ""
    done
    log "All sprites provisioned."
elif [[ "$1" == "--help" ]] || [[ "$1" == "-h" ]]; then
    usage
else
    trap lib_cleanup EXIT
    prepare_settings
    provision_sprite "$1"
fi
