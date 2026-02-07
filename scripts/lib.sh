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
WORKSPACE="$REMOTE_HOME/workspace"
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

# Restrict composition paths to this repo's compositions directory.
set_composition_path() {
    local input="$1"
    local candidate="$input"
    local allowed_root
    local resolved_parent
    local resolved_path

    if ! allowed_root="$(cd "$ROOT_DIR/compositions" 2>/dev/null && pwd -P)"; then
        err "Unable to resolve compositions directory under $ROOT_DIR"
        return 1
    fi

    if [[ "$candidate" != /* ]]; then
        candidate="$ROOT_DIR/$candidate"
    fi

    if ! resolved_parent="$(cd "$(dirname "$candidate")" 2>/dev/null && pwd -P)"; then
        err "Invalid composition path '$input'"
        return 1
    fi
    resolved_path="$resolved_parent/$(basename "$candidate")"

    if [[ "$resolved_path" != "$allowed_root"/* ]]; then
        err "Invalid composition path '$input'. Must be within $allowed_root"
        return 1
    fi

    COMPOSITION="$resolved_path"
}

lib_cleanup() {
    if [[ -n "$RENDERED_SETTINGS" && -f "$RENDERED_SETTINGS" ]]; then
        rm -f "$RENDERED_SETTINGS"
    fi
}

prepare_settings() {
    local token="${ANTHROPIC_AUTH_TOKEN:-}"
    RENDERED_SETTINGS=""

    if [[ -z "$token" ]]; then
        err "ANTHROPIC_AUTH_TOKEN is required"
        err "Export it in your shell before running this script."
        exit 1
    fi

    RENDERED_SETTINGS="$(umask 077 && mktemp)"
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

    while IFS= read -r file; do
        local rel="${file#"$local_dir"/}"
        local dest="$remote_dir/$rel"
        local parent
        parent="$(dirname "$dest")"
        "$SPRITE_CLI" exec -o "$ORG" -s "$sprite_name" -- mkdir -p "$parent"
        upload_file "$sprite_name" "$file" "$dest"
    done < <(find "$local_dir" -type f)
}

# Fall back to sprite definitions on disk when composition lookups are unavailable.
fallback_sprite_names() {
    local found=0
    local definition

    for definition in "$SPRITES_DIR"/*.md; do
        if [[ ! -e "$definition" ]]; then
            continue
        fi
        basename "$definition" .md
        found=1
    done

    if [[ "$found" -eq 0 ]]; then
        err "No sprite definitions found in $SPRITES_DIR"
        return 1
    fi
}

fallback_or_fail() {
    local strict="$1"
    local reason="$2"

    err "$reason"
    if [[ "$strict" == "true" ]]; then
        return 1
    fi

    err "Falling back to sprite definitions in $SPRITES_DIR"
    fallback_sprite_names
}

# List sprite names from the active composition YAML.
# Requires yq (mikefarah/yq) and a valid composition file.
composition_sprites() {
    local strict=false
    local sprites_from_composition=""

    if [[ "${1:-}" == "--strict" ]]; then
        strict=true
    fi

    if ! set_composition_path "$COMPOSITION"; then
        fallback_or_fail "$strict" "Invalid composition path: $COMPOSITION"
        return
    fi

    if [[ ! -f "$COMPOSITION" ]]; then
        fallback_or_fail "$strict" "Composition file not found: $COMPOSITION"
        return
    fi
    if ! command -v yq &>/dev/null; then
        fallback_or_fail "$strict" "yq is required but not installed (https://github.com/mikefarah/yq)"
        return
    fi

    if ! sprites_from_composition="$(yq '.sprites | keys | .[]' "$COMPOSITION" 2>/dev/null)"; then
        fallback_or_fail "$strict" "Failed to parse composition file: $COMPOSITION"
        return
    fi

    if ! grep -q '[^[:space:]]' <<< "$sprites_from_composition"; then
        fallback_or_fail "$strict" "No sprites found in composition: $COMPOSITION"
        return
    fi

    printf '%s\n' "$sprites_from_composition"
}

# Push base config (CLAUDE.md, hooks, skills, commands, settings) to a sprite.
# Single source of truth for what config artifacts get uploaded.
push_config() {
    local name="$1"
    upload_file "$name" "$BASE_DIR/CLAUDE.md" "$WORKSPACE/CLAUDE.md"
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
