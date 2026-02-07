#!/usr/bin/env bash
# Shared functions for bitterblossom scripts.
#
# Source this file; don't execute it directly.
#   source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"

set -euo pipefail

LIB_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(dirname "$LIB_DIR")"
SPRITES_DIR="$ROOT_DIR/sprites"
BASE_DIR="$ROOT_DIR/base"
SPRITE_CLI="${SPRITE_CLI:-sprite}"
ORG="${FLY_ORG:-misty-step}"
REMOTE_HOME="/home/sprite"
COMPOSITION="${COMPOSITION:-$ROOT_DIR/compositions/v1.yaml}"

# Rendered settings tempfile (cleaned up via lib_cleanup)
RENDERED_SETTINGS=""
SETTINGS_PATH="$BASE_DIR/settings.json"

log() { echo "[bitterblossom${LOG_PREFIX:+:$LOG_PREFIX}] $*"; }
err() { echo "[bitterblossom${LOG_PREFIX:+:$LOG_PREFIX}] ERROR: $*" >&2; }

# Validate sprite name: lowercase alphanumeric + hyphens
validate_sprite_name() {
    local name="$1"
    if [[ ! "$name" =~ ^[a-z][a-z0-9-]*$ ]]; then
        err "Invalid sprite name '$name'. Use lowercase alphanumeric + hyphens."
        return 1
    fi
}

lib_cleanup() {
    if [[ -n "$RENDERED_SETTINGS" && -f "$RENDERED_SETTINGS" ]]; then
        rm -f "$RENDERED_SETTINGS"
    fi
}

prepare_settings() {
    local token="${ANTHROPIC_AUTH_TOKEN:-}"
    if [[ -z "$token" ]]; then
        err "ANTHROPIC_AUTH_TOKEN is required"
        err "Export it in your shell before running this script."
        exit 1
    fi

    RENDERED_SETTINGS="$(mktemp)"
    chmod 600 "$RENDERED_SETTINGS"
    _BB_TOKEN="$token" python3 - "$BASE_DIR/settings.json" "$RENDERED_SETTINGS" <<'PY'
import json
import os
import sys

source_path, out_path = sys.argv[1:]
token = os.environ["_BB_TOKEN"]
with open(source_path, "r", encoding="utf-8") as source_file:
    settings = json.load(source_file)

settings.setdefault("env", {})["ANTHROPIC_AUTH_TOKEN"] = token

with open(out_path, "w", encoding="utf-8") as out_file:
    json.dump(settings, out_file, indent=2)
    out_file.write("\n")
PY
    SETTINGS_PATH="$RENDERED_SETTINGS"
}

# Upload a single file to a sprite
upload_file() {
    local sprite_name="$1"
    local local_path="$2"
    local remote_path="$3"

    "$SPRITE_CLI" exec -o "$ORG" -s "$sprite_name" \
        -file "$local_path:$remote_path" \
        -- echo "uploaded: $remote_path"
}

# Upload a directory recursively to a sprite
upload_dir() {
    local sprite_name="$1"
    local local_dir="$2"
    local remote_dir="$3"

    "$SPRITE_CLI" exec -o "$ORG" -s "$sprite_name" -- mkdir -p "$remote_dir"

    while read -r file; do
        local rel="${file#"$local_dir"/}"
        local dest="$remote_dir/$rel"
        local parent
        parent="$(dirname "$dest")"
        "$SPRITE_CLI" exec -o "$ORG" -s "$sprite_name" -- mkdir -p "$parent"
        upload_file "$sprite_name" "$file" "$dest"
    done < <(find "$local_dir" -type f)
}

# List sprite names from the active composition YAML.
# Requires yq (mikefarah/yq) and a valid composition file.
composition_sprites() {
    if [[ ! -f "$COMPOSITION" ]]; then
        err "Composition file not found: $COMPOSITION"
        return 1
    fi
    if ! command -v yq &>/dev/null; then
        err "yq is required but not installed (https://github.com/mikefarah/yq)"
        return 1
    fi
    yq '.sprites | keys | .[]' "$COMPOSITION"
}

# Push base config (CLAUDE.md, hooks, skills, commands, settings) to a sprite.
# Single source of truth for what config artifacts get uploaded.
push_config() {
    local name="$1"
    upload_file "$name" "$BASE_DIR/CLAUDE.md" "$REMOTE_HOME/workspace/CLAUDE.md"
    upload_dir "$name" "$BASE_DIR/hooks" "$REMOTE_HOME/.claude/hooks"
    upload_dir "$name" "$BASE_DIR/skills" "$REMOTE_HOME/.claude/skills"
    upload_dir "$name" "$BASE_DIR/commands" "$REMOTE_HOME/.claude/commands"
    upload_file "$name" "$SETTINGS_PATH" "$REMOTE_HOME/.claude/settings.json"
}

# Check if a sprite already exists
sprite_exists() {
    local name="$1"
    "$SPRITE_CLI" list -o "$ORG" 2>/dev/null | grep -qx "$name"
}
