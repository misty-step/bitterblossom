#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$SCRIPT_DIR/lib_bb.sh"

usage() {
    local exit_code="${1:-1}"
    cat <<'USAGE'
Usage: ./scripts/provision.sh [--composition <path>] <sprite-name|--all>

  sprite-name              Name of sprite (matches sprites/<name>.md)
  --all                    Provision all sprites from current composition
  --composition <path>     Use specific composition YAML (default: compositions/v1.yaml)
USAGE
    exit "$exit_code"
}

args=()
has_all=false
target_count=0
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
        --all)
            has_all=true
            args+=("$1")
            shift
            ;;
        --*)
            args+=("$1")
            shift
            ;;
        *)
            target_count=$((target_count + 1))
            args+=("$1")
            shift
            ;;
    esac
done

if [[ ${#args[@]} -eq 0 ]]; then
    usage 1
fi

if [[ "$has_all" == true && "$target_count" -gt 0 ]]; then
    echo "error: use either --all or explicit sprite names, not both" >&2
    usage 1
fi

bb_exec provision "${args[@]}"
