#!/usr/bin/env bash
set -euo pipefail

# Sync config updates to running sprites.
#
# Pushes updated base config, hooks, skills, commands, and optionally
# sprite persona definitions to running sprites.
#
# Usage:
#   ./scripts/sync.sh                  # Sync all sprites
#   ./scripts/sync.sh <sprite>         # Sync specific sprite
#   ./scripts/sync.sh --base-only      # Only sync base config (no persona)
#   ./scripts/sync.sh --provider <p>   # Use specific provider settings

LOG_PREFIX="sync"
source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"

BASE_ONLY=false
SYNC_PROVIDER=""
SYNC_MODEL=""

sync_sprite() {
    local name="$1"
    local provider="${2:-}"
    local model="${3:-}"

    validate_sprite_name "$name"

    log "=== Syncing: $name ==="

    if ! sprite_exists "$name"; then
        err "Sprite '$name' does not exist. Run provision.sh first."
        return 1
    fi

    # Get provider configuration if not specified
    if [[ -z "$provider" ]]; then
        local provider_info
        provider_info="$(get_provider_for_sprite "$name")"
        IFS=$'\t' read -r provider model <<< "$provider_info"
    fi

    log "Using provider: $provider${model:+ (model: $model)}"

    # Prepare settings with provider-specific configuration
    prepare_settings_with_provider "$provider" "$model"

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
        --provider)
            if [[ -z "${2:-}" ]]; then
                err "--provider requires a value"
                exit 1
            fi
            SYNC_PROVIDER="$2"
            shift 2
            ;;
        --model)
            if [[ -z "${2:-}" ]]; then
                err "--model requires a value"
                exit 1
            fi
            SYNC_MODEL="$2"
            shift 2
            ;;
        --composition)
            if [[ -z "${2:-}" ]]; then
                err "--composition requires a value"
                exit 1
            fi
            set_composition_path "$2" || exit 1
            shift 2
            ;;
        --help|-h)
            echo "Usage: $0 [--base-only] [--provider <name>] [--model <model>] [--composition <path>] [sprite-name ...]"
            echo ""
            echo "  --base-only          Only sync shared base config (skip persona definitions)"
            echo "  --provider <name>    LLM provider: moonshot, openrouter-kimi, openrouter-claude"
            echo "  --model <model>      Model identifier (e.g., kimi-k2.5, anthropic/claude-opus-4)"
            echo "  --composition <path> Use specific composition YAML (default: compositions/v1.yaml)"
            echo "  sprite-name          Sync specific sprite(s). Default: all from composition."
            echo ""
            echo "Environment:"
            echo "  ANTHROPIC_AUTH_TOKEN     API key for Moonshot or OpenRouter"
            echo "  BB_OPENROUTER_API_KEY    Alternative OpenRouter API key"
            exit 0
            ;;
        *) TARGETS+=("$1"); shift ;;
    esac
done

trap lib_cleanup EXIT

if [[ ${#TARGETS[@]} -eq 0 ]]; then
    log "Syncing sprites from composition: $COMPOSITION"
    sprite_list=$(composition_sprites --strict) || exit 1
    if [[ -z "$sprite_list" ]]; then
        err "No sprites found in composition: $COMPOSITION"
        exit 1
    fi
    while IFS= read -r name; do
        sync_sprite "$name" "$SYNC_PROVIDER" "$SYNC_MODEL"
        echo ""
    done <<< "$sprite_list"
    log "All sprites synced."
else
    for name in "${TARGETS[@]}"; do
        sync_sprite "$name" "$SYNC_PROVIDER" "$SYNC_MODEL"
        echo ""
    done
fi
