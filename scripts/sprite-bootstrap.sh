#!/usr/bin/env bash
set -euo pipefail

# sprite-bootstrap.sh — Idempotent sprite environment setup
#
# Usage:
#   ./scripts/sprite-bootstrap.sh
#   ./scripts/sprite-bootstrap.sh --agent-source /tmp/sprite-agent.sh

usage() {
    local exit_code="${1:-0}"
    cat <<'EOF'
Usage: sprite-bootstrap.sh [--agent-source <path>] [--version <stamp>]

Options:
  --agent-source <path>  Source path for sprite-agent.sh (default: /tmp/sprite-agent.sh)
  --version <stamp>      Version stamp written to ~/.sprite-tools-version
  --help, -h             Show this help text
EOF
    exit "$exit_code"
}

AGENT_SOURCE="/tmp/sprite-agent.sh"
TOOLS_VERSION="${TOOLS_VERSION:-sprite-tools=0.2.0}"

while [[ $# -gt 0 ]]; do
    case "$1" in
        --agent-source)
            [[ -z "${2:-}" ]] && { echo "ERROR: --agent-source requires a value" >&2; usage 1; }
            AGENT_SOURCE="$2"
            shift 2
            ;;
        --version)
            [[ -z "${2:-}" ]] && { echo "ERROR: --version requires a value" >&2; usage 1; }
            TOOLS_VERSION="$2"
            shift 2
            ;;
        --help|-h) usage 0 ;;
        *) echo "ERROR: Unknown argument: $1" >&2; usage 1 ;;
    esac
done

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
if [[ ! -f "$AGENT_SOURCE" && -f "$SCRIPT_DIR/sprite-agent.sh" ]]; then
    AGENT_SOURCE="$SCRIPT_DIR/sprite-agent.sh"
fi

if [[ ! -f "$AGENT_SOURCE" ]]; then
    echo "ERROR: sprite-agent source not found: $AGENT_SOURCE" >&2
    exit 1
fi

mkdir -p \
    "$HOME/workspace/repos" \
    "$HOME/workspace/logs" \
    "$HOME/workspace/checkpoints" \
    "$HOME/.local/bin" \
    "$HOME/.config/sprite-agent"

missing_packages=()
command -v rg >/dev/null 2>&1 || missing_packages+=("ripgrep")
if ! command -v fd >/dev/null 2>&1 && ! command -v fdfind >/dev/null 2>&1; then
    missing_packages+=("fd-find")
fi
command -v tmux >/dev/null 2>&1 || missing_packages+=("tmux")
command -v htop >/dev/null 2>&1 || missing_packages+=("htop")
command -v tree >/dev/null 2>&1 || missing_packages+=("tree")

if (( ${#missing_packages[@]} > 0 )); then
    if command -v sudo >/dev/null 2>&1; then
        sudo apt-get update -y
        sudo apt-get install -y "${missing_packages[@]}"
    else
        apt-get update -y
        apt-get install -y "${missing_packages[@]}"
    fi
fi

if command -v fdfind >/dev/null 2>&1 && ! command -v fd >/dev/null 2>&1; then
    ln -sf "$(command -v fdfind)" "$HOME/.local/bin/fd"
fi

install -m 0755 "$AGENT_SOURCE" "$HOME/.local/bin/sprite-agent"

PATH_LINE='export PATH="$HOME/.local/bin:$PATH"'
touch "$HOME/.bashrc"
if ! grep -Fqx "$PATH_LINE" "$HOME/.bashrc"; then
    printf '\n%s\n' "$PATH_LINE" >> "$HOME/.bashrc"
fi

{
    printf '%s\n' "$TOOLS_VERSION"
    printf 'updated_at=%s\n' "$(date -u +%Y-%m-%dT%H:%M:%SZ)"
} > "$HOME/.sprite-tools-version"

echo "sprite-bootstrap complete"
