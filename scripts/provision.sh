#!/usr/bin/env bash
set -euo pipefail

# Provision a sprite on Fly.io from its definition
#
# Usage:
#   ./scripts/provision.sh <sprite-name>    # Provision single sprite
#   ./scripts/provision.sh --all            # Provision all sprites from v1 composition

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(dirname "$SCRIPT_DIR")"
SPRITES_DIR="$ROOT_DIR/sprites"
BASE_DIR="$ROOT_DIR/base"

usage() {
    echo "Usage: $0 <sprite-name|--all>"
    echo ""
    echo "  sprite-name   Name of sprite (matches sprites/<name>.md)"
    echo "  --all         Provision all sprites from current composition"
    echo ""
    echo "Examples:"
    echo "  $0 bramblecap"
    echo "  $0 --all"
    exit 1
}

provision_sprite() {
    local name="$1"
    local definition="$SPRITES_DIR/${name}.md"

    if [[ ! -f "$definition" ]]; then
        echo "ERROR: No sprite definition found at $definition"
        exit 1
    fi

    echo "=== Provisioning sprite: $name ==="

    # TODO: Implement Fly.io machine creation
    # This is a placeholder for the actual provisioning logic.
    # Steps will include:
    # 1. Create Fly.io machine with appropriate specs
    # 2. Copy base/ config (CLAUDE.md, hooks, skills, commands, settings)
    # 3. Copy sprite definition as the agent prompt
    # 4. Set up MEMORY.md
    # 5. Configure Claude Code with base/settings.json
    # 6. Verify sprite health

    echo "  Definition: $definition"
    echo "  Base config: $BASE_DIR/"
    echo "  Skills: $(ls "$BASE_DIR/skills/" | tr '\n' ' ')"
    echo "  Hooks: $(ls "$BASE_DIR/hooks/"*.py 2>/dev/null | xargs -I{} basename {} | tr '\n' ' ')"
    echo ""
    echo "  [PLACEHOLDER] Fly.io machine creation not yet implemented"
    echo "  TODO: fly machine run ... --name sprite-${name}"
    echo ""
    echo "=== Done: $name ==="
}

if [[ $# -eq 0 ]]; then
    usage
fi

if [[ "$1" == "--all" ]]; then
    echo "Provisioning all sprites..."
    for def in "$SPRITES_DIR"/*.md; do
        name="$(basename "$def" .md)"
        provision_sprite "$name"
        echo ""
    done
    echo "All sprites provisioned."
elif [[ "$1" == "--help" ]] || [[ "$1" == "-h" ]]; then
    usage
else
    provision_sprite "$1"
fi
