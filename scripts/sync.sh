#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$SCRIPT_DIR/lib_bb.sh"

usage() {
    local exit_code="${1:-1}"
    cat <<'USAGE'
Usage: ./scripts/sync.sh [--base-only] [--composition <path>] [sprite-name ...]

  --base-only          Only sync shared base config (skip persona definitions)
  --composition <path> Use specific composition YAML (default: compositions/v1.yaml)
  sprite-name          Sync specific sprite(s). Default: all from composition.
USAGE
    exit "$exit_code"
}

args=()
while [[ $# -gt 0 ]]; do
    case "$1" in
        --help|-h)
            usage 0
            ;;
        --composition)
            if [[ -z "${2:-}" ]]; then
                echo "error: --composition requires a value" >&2
                usage 1
            fi
            args+=("$1" "$2")
            shift 2
            ;;
        *)
            args+=("$1")
            shift
            ;;
    esac
done

if [[ ${#args[@]} -eq 0 ]]; then
    bb_exec sync
else
    bb_exec sync "${args[@]}"
fi
