#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$SCRIPT_DIR/lib_bb.sh"

usage() {
    local exit_code="${1:-1}"
    cat <<'USAGE'
Usage: ./scripts/status.sh [--composition <path>] [sprite-name]

  No args: fleet overview
  --composition <path>  Use specific composition YAML
  sprite-name: detailed status for one sprite
USAGE
    exit "$exit_code"
}

args=()
format_set=false
target=""

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
        --format)
            if [[ -z "${2:-}" ]]; then
                echo "error: --format requires a value" >&2
                usage 1
            fi
            format_set=true
            args+=("$1" "$2")
            shift 2
            ;;
        --json)
            format_set=true
            args+=("$1")
            shift
            ;;
        --*)
            args+=("$1")
            shift
            ;;
        *)
            if [[ -n "$target" ]]; then
                echo "error: only one sprite name can be provided" >&2
                usage 1
            fi
            target="$1"
            shift
            ;;
    esac
done

if [[ "$format_set" == false ]]; then
    if [[ ${#args[@]} -eq 0 ]]; then
        args=(--format text)
    else
        args=(--format text "${args[@]}")
    fi
fi

if [[ -n "$target" ]]; then
    args+=("$target")
fi

bb_exec status "${args[@]}"
