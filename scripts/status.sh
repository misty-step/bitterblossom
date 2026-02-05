#!/usr/bin/env bash
set -euo pipefail

# Show status of the sprite fleet.
#
# Usage:
#   ./scripts/status.sh              # Fleet overview
#   ./scripts/status.sh <sprite>     # Detailed sprite status

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(dirname "$SCRIPT_DIR")"
SPRITES_DIR="$ROOT_DIR/sprites"
SPRITE_CLI="${SPRITE_CLI:-sprite}"
ORG="${FLY_ORG:-misty-step}"
REMOTE_HOME="/home/sprite"

log() { echo "$*"; }

fleet_status() {
    echo "=== Bitterblossom Fleet Status ==="
    echo ""

    # Get live sprite list from Fly.io
    local live_sprites
    live_sprites=$("$SPRITE_CLI" api -o "$ORG" /sprites 2>/dev/null | \
        python3 -c "import sys,json; data=json.load(sys.stdin); [print(f\"{s['name']}\t{s['status']}\t{s.get('url','n/a')}\") for s in data.get('sprites',[])]" 2>/dev/null || echo "")

    if [[ -z "$live_sprites" ]]; then
        echo "No sprites found (or API call failed)."
        echo ""
        echo "Defined sprites (not yet provisioned):"
        for def in "$SPRITES_DIR"/*.md; do
            echo "  - $(basename "$def" .md)"
        done
        return
    fi

    # Show live sprites
    printf "%-15s %-8s %s\n" "SPRITE" "STATUS" "URL"
    printf "%-15s %-8s %s\n" "------" "------" "---"
    echo "$live_sprites" | while IFS=$'\t' read -r name status url; do
        printf "%-15s %-8s %s\n" "$name" "$status" "$url"
    done

    # Show defined but not provisioned
    echo ""
    echo "Defined sprites:"
    for def in "$SPRITES_DIR"/*.md; do
        local name
        name="$(basename "$def" .md)"
        if echo "$live_sprites" | grep -q "^$name"; then
            echo "  ✓ $name (provisioned)"
        else
            echo "  ○ $name (not provisioned)"
        fi
    done

    # Show checkpoints
    echo ""
    echo "Checkpoints:"
    echo "$live_sprites" | while IFS=$'\t' read -r name status url; do
        local checkpoints
        checkpoints=$("$SPRITE_CLI" checkpoint list -o "$ORG" -s "$name" 2>/dev/null || echo "  (none)")
        echo "  $name: $checkpoints"
    done
}

sprite_detail() {
    local name="$1"
    echo "=== Sprite: $name ==="
    echo ""

    # API info
    "$SPRITE_CLI" api -o "$ORG" -s "$name" / 2>/dev/null | python3 -c "
import sys, json
data = json.load(sys.stdin)
for k, v in data.items():
    if isinstance(v, dict):
        print(f'{k}:')
        for kk, vv in v.items():
            print(f'  {kk}: {vv}')
    else:
        print(f'{k}: {v}')
" 2>/dev/null || echo "(API call failed)"

    echo ""

    # Check workspace contents
    echo "Workspace:"
    "$SPRITE_CLI" exec -o "$ORG" -s "$name" -- ls -la "$REMOTE_HOME/workspace/" 2>/dev/null || echo "  (no workspace)"

    echo ""

    # Show MEMORY.md summary
    echo "MEMORY.md (first 20 lines):"
    "$SPRITE_CLI" exec -o "$ORG" -s "$name" -- head -20 "$REMOTE_HOME/workspace/MEMORY.md" 2>/dev/null || echo "  (no MEMORY.md)"

    echo ""

    # Checkpoints
    echo "Checkpoints:"
    "$SPRITE_CLI" checkpoint list -o "$ORG" -s "$name" 2>/dev/null || echo "  (none)"
}

if [[ $# -eq 0 ]]; then
    fleet_status
elif [[ "$1" == "--help" || "$1" == "-h" ]]; then
    echo "Usage: $0 [sprite-name]"
    echo ""
    echo "  No args: fleet overview"
    echo "  sprite-name: detailed status for one sprite"
    exit 0
else
    sprite_detail "$1"
fi
