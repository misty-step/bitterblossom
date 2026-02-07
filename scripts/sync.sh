#!/usr/bin/env bash
set -euo pipefail

# Sync config updates to running sprites.
#
# Pushes updated base config, hooks, skills, commands, and optionally
# sprite persona definitions to running sprites.
#
# Usage:
#   ./scripts/sync.sh              # Sync all sprites
#   ./scripts/sync.sh <sprite>     # Sync specific sprite
#   ./scripts/sync.sh --base-only  # Only sync base config (no persona)

LOG_PREFIX="sync" source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"

BASE_ONLY=false

sync_sprite() {
    local name="$1"

    validate_sprite_name "$name"

    log "=== Syncing: $name ==="

    if ! sprite_exists "$name"; then
        err "Sprite '$name' does not exist. Run provision.sh first."
        return 1
    fi

    # Sync base config (CLAUDE.md, hooks, skills, commands, settings)
    log "Syncing base config..."
    push_config "$name"

    # Sync persona definition (unless --base-only)
    if [[ "$BASE_ONLY" == false ]]; then
        local definition="$SPRITES_DIR/${name}.md"
        if [[ -f "$definition" ]]; then
            log "Syncing persona definition..."
            upload_file "$name" "$definition" "$REMOTE_HOME/workspace/PERSONA.md"
        else
            log "No persona definition found at $definition, skipping"
        fi
    fi

    log "=== Done: $name ==="
}

# Parse args
TARGETS=()
while [[ $# -gt 0 ]]; do
    case "$1" in
        --base-only) BASE_ONLY=true; shift ;;
        --composition)
            if [[ -z "${2:-}" ]]; then
                err "--composition requires a value"
                exit 1
            fi
            set_composition_path "$2" || exit 1
            shift 2
            ;;
        --help|-h)
            echo "Usage: $0 [--base-only] [--composition <path>] [sprite-name ...]"
            echo ""
            echo "  --base-only          Only sync shared base config (skip persona definitions)"
            echo "  --composition <path> Use specific composition YAML (default: compositions/v1.yaml)"
            echo "  sprite-name          Sync specific sprite(s). Default: all from composition."
            exit 0
            ;;
        *) TARGETS+=("$1"); shift ;;
    esac
done

trap lib_cleanup EXIT
prepare_settings

if [[ ${#TARGETS[@]} -eq 0 ]]; then
    log "Syncing sprites from composition: $COMPOSITION"
    sprite_list=$(composition_sprites) || exit 1
    if [[ -z "$sprite_list" ]]; then
        err "No sprites found in composition: $COMPOSITION"
        exit 1
    fi
    while IFS= read -r name; do
        sync_sprite "$name"
        echo ""
    done <<< "$sprite_list"
    log "All sprites synced."
else
    for name in "${TARGETS[@]}"; do
        sync_sprite "$name"
        echo ""
    done
fi
