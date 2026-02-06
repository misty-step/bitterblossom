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

    # Step 3: Upload base CLAUDE.md as the project-level config
    log "Uploading base CLAUDE.md..."
    upload_file "$name" "$BASE_DIR/CLAUDE.md" "$REMOTE_HOME/workspace/CLAUDE.md"

    # Step 4: Upload sprite persona definition
    log "Uploading persona: $name.md..."
    upload_file "$name" "$definition" "$REMOTE_HOME/workspace/PERSONA.md"

    # Step 5: Upload hooks
    log "Uploading hooks..."
    upload_dir "$name" "$BASE_DIR/hooks" "$REMOTE_HOME/.claude/hooks"

    # Step 6: Upload skills
    log "Uploading skills..."
    upload_dir "$name" "$BASE_DIR/skills" "$REMOTE_HOME/.claude/skills"

    # Step 7: Upload commands
    log "Uploading commands..."
    upload_dir "$name" "$BASE_DIR/commands" "$REMOTE_HOME/.claude/commands"

    # Step 8: Upload settings.json (Claude Code config with hooks + env)
    log "Uploading Claude Code settings..."
    upload_file "$name" "$SETTINGS_PATH" "$REMOTE_HOME/.claude/settings.json"

    # Step 9: Create initial MEMORY.md
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

    # Step 10: Set up git config for shared account
    log "Configuring git..."
    "$SPRITE_CLI" exec -o "$ORG" -s "$name" -- bash -c \
        "git config --global user.name '$name (bitterblossom sprite)' && \
         git config --global user.email 'kaylee@mistystep.io'"

    # Step 11: Create initial checkpoint
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
