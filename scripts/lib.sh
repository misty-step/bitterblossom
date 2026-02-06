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

    find "$local_dir" -type f | while read -r file; do
        local rel="${file#"$local_dir"/}"
        local dest="$remote_dir/$rel"
        local parent
        parent="$(dirname "$dest")"
        "$SPRITE_CLI" exec -o "$ORG" -s "$sprite_name" -- mkdir -p "$parent"
        upload_file "$sprite_name" "$file" "$dest"
    done
}

# Check if a sprite already exists
sprite_exists() {
    local name="$1"
    "$SPRITE_CLI" list -o "$ORG" 2>/dev/null | grep -qx "$name"
}
