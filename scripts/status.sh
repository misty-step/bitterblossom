#!/usr/bin/env bash
set -euo pipefail

# Show status of the sprite fleet.
#
# Usage:
#   ./scripts/status.sh              # Fleet overview
#   ./scripts/status.sh <sprite>     # Detailed sprite status

LOG_PREFIX="status" source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"

fleet_status() {
    echo "=== Bitterblossom Fleet Status ==="
    echo ""

    # Get live sprite list from Fly.io
    local live_sprites
    live_sprites=$("$SPRITE_CLI" api -o "$ORG" /sprites 2>/dev/null | \
        python3 -c "import sys,json; data=json.load(sys.stdin); [print(f\"{s['name']}\t{s['status']}\t{s.get('url','n/a')}\") for s in data.get('sprites',[])]" 2>/dev/null || echo "")

    local sprite_list
    sprite_list=$(composition_sprites) || sprite_list=""

    if [[ -z "$live_sprites" ]]; then
        echo "No sprites found (or API call failed)."
        echo ""
        echo "Composition sprites ($COMPOSITION):"
        if [[ -n "$sprite_list" ]]; then
            while IFS= read -r name; do
                validate_sprite_name "$name" || continue
                echo "  - $name (not provisioned)"
            done <<< "$sprite_list"
        fi
        return
    fi

    # Show live sprites
    printf "%-15s %-8s %s\n" "SPRITE" "STATUS" "URL"
    printf "%-15s %-8s %s\n" "------" "------" "---"
    echo "$live_sprites" | while IFS=$'\t' read -r name status url; do
        printf "%-15s %-8s %s\n" "$name" "$status" "$url"
    done

    # Show composition sprites vs provisioned
    echo ""
    if [[ -n "$sprite_list" ]]; then
        echo "Composition sprites ($COMPOSITION):"
        while IFS= read -r name; do
            validate_sprite_name "$name" || continue
            if echo "$live_sprites" | grep -qF "${name}	"; then
                echo "  ✓ $name (provisioned)"
            else
                echo "  ○ $name (not provisioned)"
            fi
        done <<< "$sprite_list"
    fi

    # Show orphan sprites (live but not in composition)
    if [[ -n "$sprite_list" ]]; then
        local orphans=""
        while IFS=$'\t' read -r name status url; do
            if ! echo "$sprite_list" | grep -qxF "$name"; then
                orphans+="  ? $name ($status, not in composition)"$'\n'
            fi
        done <<< "$live_sprites"
        if [[ -n "$orphans" ]]; then
            echo ""
            echo "Orphan sprites (live but not in composition):"
            printf "%s" "$orphans"
        fi
    fi

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
    validate_sprite_name "$name"
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

# Parse args
TARGET=""
while [[ $# -gt 0 ]]; do
    case "$1" in
        --composition)
            if [[ -z "${2:-}" ]]; then
                err "--composition requires a value"
                exit 1
            fi
            COMPOSITION="$2"
            shift 2
            ;;
        --help|-h)
            echo "Usage: $0 [--composition <path>] [sprite-name]"
            echo ""
            echo "  No args: fleet overview"
            echo "  --composition <path>  Use specific composition YAML"
            echo "  sprite-name: detailed status for one sprite"
            exit 0
            ;;
        *) TARGET="$1"; shift ;;
    esac
done

if [[ -z "$TARGET" ]]; then
    fleet_status
else
    sprite_detail "$TARGET"
fi
