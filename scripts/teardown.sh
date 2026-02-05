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

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(dirname "$SCRIPT_DIR")"
SPRITES_DIR="$ROOT_DIR/sprites"
ARCHIVE_DIR="$ROOT_DIR/observations/archives"
SPRITE_CLI="${SPRITE_CLI:-sprite}"
ORG="${FLY_ORG:-misty-step}"
REMOTE_HOME="/home/sprite"

FORCE=false

log() { echo "[bitterblossom:teardown] $*"; }
err() { echo "[bitterblossom:teardown] ERROR: $*" >&2; }

sprite_exists() {
    local name="$1"
    "$SPRITE_CLI" list -o "$ORG" 2>/dev/null | grep -qx "$name"
}

teardown_sprite() {
    local name="$1"
    local timestamp
    timestamp="$(date -u +%Y%m%dT%H%M%SZ)"
    local archive_path="$ARCHIVE_DIR/${name}-${timestamp}"

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

    # Export the sprite's modified settings.json
    if "$SPRITE_CLI" exec -o "$ORG" -s "$name" -- cat "$REMOTE_HOME/.claude/settings.json" \
        > "$archive_path/settings.json" 2>/dev/null; then
        log "Exported settings.json → $archive_path/settings.json"
    else
        log "No settings.json found"
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
