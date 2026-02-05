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

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(dirname "$SCRIPT_DIR")"
SPRITES_DIR="$ROOT_DIR/sprites"
BASE_DIR="$ROOT_DIR/base"
SPRITE_CLI="${SPRITE_CLI:-sprite}"
ORG="${FLY_ORG:-misty-step}"
REMOTE_HOME="/home/sprite"
SETTINGS_PATH="$BASE_DIR/settings.json"
RENDERED_SETTINGS=""

BASE_ONLY=false

log() { echo "[bitterblossom:sync] $*"; }
err() { echo "[bitterblossom:sync] ERROR: $*" >&2; }

cleanup() {
    if [[ -n "$RENDERED_SETTINGS" && -f "$RENDERED_SETTINGS" ]]; then
        rm -f "$RENDERED_SETTINGS"
    fi
}

prepare_settings() {
    local token="${ANTHROPIC_AUTH_TOKEN:-}"
    if [[ -z "$token" ]]; then
        err "ANTHROPIC_AUTH_TOKEN is required to sync settings.json"
        err "Export it in your shell before running this script."
        exit 1
    fi

    RENDERED_SETTINGS="$(mktemp)"
    python3 - "$BASE_DIR/settings.json" "$RENDERED_SETTINGS" "$token" <<'PY'
import json
import sys

source_path, out_path, token = sys.argv[1:]
with open(source_path, "r", encoding="utf-8") as source_file:
    settings = json.load(source_file)

settings.setdefault("env", {})["ANTHROPIC_AUTH_TOKEN"] = token

with open(out_path, "w", encoding="utf-8") as out_file:
    json.dump(settings, out_file, indent=2)
    out_file.write("\n")
PY
    SETTINGS_PATH="$RENDERED_SETTINGS"
}

upload_file() {
    local sprite_name="$1" local_path="$2" remote_path="$3"
    "$SPRITE_CLI" exec -o "$ORG" -s "$sprite_name" \
        -file "$local_path:$remote_path" \
        -- echo "synced: $remote_path"
}

upload_dir() {
    local sprite_name="$1" local_dir="$2" remote_dir="$3"
    "$SPRITE_CLI" exec -o "$ORG" -s "$sprite_name" -- mkdir -p "$remote_dir"
    find "$local_dir" -type f | while read -r file; do
        local rel="${file#"$local_dir"/}"
        local dest="$remote_dir/$rel"
        local parent
        parent="$(dirname "$dest")"
        "$SPRITE_CLI" exec -o "$ORG" -s "$sprite_name" -- mkdir -p "$parent"
        upload_file "$sprite_name" "$file" "$dest"
    done
}

sprite_exists() {
    local name="$1"
    "$SPRITE_CLI" list -o "$ORG" 2>/dev/null | grep -qx "$name"
}

sync_sprite() {
    local name="$1"
    log "=== Syncing: $name ==="

    if ! sprite_exists "$name"; then
        err "Sprite '$name' does not exist. Run provision.sh first."
        return 1
    fi

    # Sync base CLAUDE.md
    log "Syncing base CLAUDE.md..."
    upload_file "$name" "$BASE_DIR/CLAUDE.md" "$REMOTE_HOME/workspace/CLAUDE.md"

    # Sync hooks
    log "Syncing hooks..."
    upload_dir "$name" "$BASE_DIR/hooks" "$REMOTE_HOME/.claude/hooks"

    # Sync skills
    log "Syncing skills..."
    upload_dir "$name" "$BASE_DIR/skills" "$REMOTE_HOME/.claude/skills"

    # Sync commands
    log "Syncing commands..."
    upload_dir "$name" "$BASE_DIR/commands" "$REMOTE_HOME/.claude/commands"

    # Sync settings.json
    log "Syncing Claude Code settings..."
    upload_file "$name" "$SETTINGS_PATH" "$REMOTE_HOME/.claude/settings.json"

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
for arg in "$@"; do
    case "$arg" in
        --base-only) BASE_ONLY=true ;;
        --help|-h)
            echo "Usage: $0 [--base-only] [sprite-name ...]"
            echo ""
            echo "  --base-only   Only sync shared base config (skip persona definitions)"
            echo "  sprite-name   Sync specific sprite(s). Default: all."
            exit 0
            ;;
        *) TARGETS+=("$arg") ;;
    esac
done

if [[ ${#TARGETS[@]} -eq 0 ]]; then
    trap cleanup EXIT
    prepare_settings
    log "Syncing all sprites..."
    for def in "$SPRITES_DIR"/*.md; do
        name="$(basename "$def" .md)"
        sync_sprite "$name"
        echo ""
    done
    log "All sprites synced."
else
    trap cleanup EXIT
    prepare_settings
    for name in "${TARGETS[@]}"; do
        sync_sprite "$name"
        echo ""
    done
fi
