#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$SCRIPT_DIR/lib_bb.sh"

usage() {
    local exit_code="${1:-1}"
    cat <<'USAGE'
Usage: ./scripts/teardown.sh [--force] <sprite-name>

  --force   Skip confirmation prompt
USAGE
    exit "$exit_code"
}

args=()
sprite_name=""

while [[ $# -gt 0 ]]; do
    case "$1" in
        --help|-h)
            usage 0
            ;;
        --force|-f)
            args+=(--force)
            shift
            ;;
        --*)
            args+=("$1")
            shift
            ;;
        *)
            if [[ -n "$sprite_name" ]]; then
                echo "error: too many arguments" >&2
                usage 1
            fi
            sprite_name="$1"
            shift
            ;;
    esac
done

if [[ -z "$sprite_name" ]]; then
    usage 1
fi

bb_exec teardown "${args[@]}" "$sprite_name"
