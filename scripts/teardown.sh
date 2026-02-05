#!/usr/bin/env bash
set -euo pipefail

# Decommission a sprite
#
# Usage:
#   ./scripts/teardown.sh <sprite-name>

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(dirname "$SCRIPT_DIR")"
SPRITES_DIR="$ROOT_DIR/sprites"

usage() {
    echo "Usage: $0 <sprite-name>"
    echo ""
    echo "  Decommission a running sprite. Preserves the definition file."
    echo ""
    echo "Examples:"
    echo "  $0 bramblecap"
    exit 1
}

if [[ $# -eq 0 ]] || [[ "$1" == "--help" ]] || [[ "$1" == "-h" ]]; then
    usage
fi

name="$1"

echo "=== Tearing down sprite: $name ==="
echo ""

# Safety check: confirm the sprite definition exists
if [[ ! -f "$SPRITES_DIR/${name}.md" ]]; then
    echo "WARNING: No sprite definition at $SPRITES_DIR/${name}.md"
    echo "Proceeding anyway (machine may still exist)..."
fi

# TODO: Implement actual teardown
# Steps:
# 1. Export MEMORY.md from sprite (preserve learnings)
# 2. Stop Claude Code process
# 3. Destroy Fly.io machine
# 4. Archive any logs

echo "  [PLACEHOLDER] Teardown not yet implemented"
echo "  TODO: fly machine destroy sprite-${name} --force"
echo ""
echo "  NOTE: Sprite definition preserved at $SPRITES_DIR/${name}.md"
echo "  NOTE: Remember to export MEMORY.md before destroying"
echo ""
echo "=== Done: $name ==="
