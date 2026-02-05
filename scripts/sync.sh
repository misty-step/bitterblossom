#!/usr/bin/env bash
set -euo pipefail

# Sync config updates to running sprite fleet
#
# Usage:
#   ./scripts/sync.sh              # Sync base config to all sprites
#   ./scripts/sync.sh <sprite>     # Sync to specific sprite

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(dirname "$SCRIPT_DIR")"
SPRITES_DIR="$ROOT_DIR/sprites"
BASE_DIR="$ROOT_DIR/base"

sync_sprite() {
    local name="$1"
    echo "=== Syncing: $name ==="

    # TODO: Implement actual sync to Fly.io machines
    # Steps:
    # 1. Push updated base/ config (CLAUDE.md, hooks, skills, settings)
    # 2. Push updated sprite definition if changed
    # 3. Restart Claude Code process to pick up changes

    echo "  [PLACEHOLDER] Sync not yet implemented"
    echo "  TODO: fly ssh console -a sprite-${name} -C 'update-config'"
    echo "=== Done: $name ==="
}

if [[ $# -eq 0 ]]; then
    echo "Syncing all sprites..."
    for def in "$SPRITES_DIR"/*.md; do
        name="$(basename "$def" .md)"
        sync_sprite "$name"
        echo ""
    done
    echo "All sprites synced."
else
    sync_sprite "$1"
fi
