#!/usr/bin/env bash
set -euo pipefail

# Decommission a sprite.
#
# Exports MEMORY.md and CLAUDE.md (sprite's self-modified config) before
# destroying the sprite. Preserves the sprite definition file.
#
# Usage:
#   ./scripts/teardown.sh <sprite-name>
#   ./scripts/teardown.sh --force <sprite-name>   # Skip confirmation

LOG_PREFIX="teardown"
source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"

ARCHIVE_DIR="$ROOT_DIR/observations/archives"
FORCE=false

teardown_sprite() {
    local name="$1"
    local timestamp
    timestamp="$(date -u +%Y%m%dT%H%M%SZ)"
    local archive_path="$ARCHIVE_DIR/${name}-${timestamp}"

    validate_sprite_name "$name"

    log "=== Tearing down sprite: $name ==="

    if ! sprite_exists "$name"; then
        err "Sprite '$name' does not exist"
        exit 1
    fi

    # Confirmation
    if [[ "$FORCE" == false ]]; then
        echo ""
        echo "  This will DESTROY sprite '$name' and its disk."
        echo "  MEMORY.md and workspace CLAUDE.md will be exported first."
        echo ""
        read -rp "  Continue? [y/N] " confirm
        if [[ "$confirm" != "y" && "$confirm" != "Y" ]]; then
            log "Aborted."
            exit 0
        fi
    fi

    # Step 1: Export learnings
    log "Exporting sprite data..."
    mkdir -p "$archive_path"

    # Export MEMORY.md
    if "$SPRITE_CLI" exec -o "$ORG" -s "$name" -- cat "$REMOTE_HOME/workspace/MEMORY.md" \
        > "$archive_path/MEMORY.md" 2>/dev/null; then
        log "Exported MEMORY.md → $archive_path/MEMORY.md"
    else
        log "No MEMORY.md found (or empty)"
    fi

    # Export the sprite's self-modified CLAUDE.md
    if "$SPRITE_CLI" exec -o "$ORG" -s "$name" -- cat "$REMOTE_HOME/workspace/CLAUDE.md" \
        > "$archive_path/CLAUDE.md" 2>/dev/null; then
        log "Exported CLAUDE.md → $archive_path/CLAUDE.md"
    else
        log "No workspace CLAUDE.md found"
    fi

    # Export the sprite's modified settings.json (strip auth token)
    if "$SPRITE_CLI" exec -o "$ORG" -s "$name" -- cat "$REMOTE_HOME/.claude/settings.json" \
        2>/dev/null | python3 -c "
import sys, json
try:
    data = json.load(sys.stdin)
    env = data.get('env', {})
    if 'ANTHROPIC_AUTH_TOKEN' in env:
        env['ANTHROPIC_AUTH_TOKEN'] = '__REDACTED__'
    json.dump(data, sys.stdout, indent=2)
except: sys.exit(1)
" > "$archive_path/settings.json"; then
        log "Exported settings.json → $archive_path/settings.json (token redacted)"
    else
        log "No settings.json found (or parse error)"
    fi

    # Step 2: Create a final checkpoint (safety net)
    log "Creating final checkpoint before destruction..."
    "$SPRITE_CLI" checkpoint create -o "$ORG" -s "$name" 2>&1 || log "Final checkpoint failed (continuing)"

    # Step 3: Destroy the sprite
    log "Destroying sprite '$name'..."
    "$SPRITE_CLI" destroy "$name" -o "$ORG" --force

    log ""
    log "Sprite '$name' destroyed."
    log "Archives saved to: $archive_path/"
    log "Sprite definition preserved at: $SPRITES_DIR/${name}.md"
    log ""
    log "=== Done: $name ==="
}

# Parse args
SPRITE_NAME=""
for arg in "$@"; do
    case "$arg" in
        --force|-f) FORCE=true ;;
        --help|-h)
            echo "Usage: $0 [--force] <sprite-name>"
            echo ""
            echo "  --force   Skip confirmation prompt"
            exit 0
            ;;
        *)
            if [[ -z "$SPRITE_NAME" ]]; then
                SPRITE_NAME="$arg"
            else
                err "Too many arguments"
                exit 1
            fi
            ;;
    esac
done

if [[ -z "$SPRITE_NAME" ]]; then
    echo "Usage: $0 [--force] <sprite-name>"
    exit 1
fi

teardown_sprite "$SPRITE_NAME"
