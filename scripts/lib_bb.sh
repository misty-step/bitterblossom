#!/usr/bin/env bash
set -euo pipefail

_bb_script_dir() {
    cd "$(dirname "${BASH_SOURCE[0]}")" && pwd
}

_bb_repo_root() {
    cd "$(_bb_script_dir)/.." && pwd
}

bb_exec() {
    local repo_root
    repo_root="$(_bb_repo_root)"

    if [[ -n "${BB_BIN:-}" ]]; then
        local bb_bin
        bb_bin="$BB_BIN"
        if [[ "$bb_bin" == */* && "$bb_bin" != /* ]]; then
            bb_bin="$PWD/$bb_bin"
        fi
        if [[ "$bb_bin" != */* ]]; then
            bb_bin="$(command -v "$bb_bin" || true)"
        fi
        if [[ -z "$bb_bin" ]]; then
            echo "error: BB_BIN=$BB_BIN could not be resolved" >&2
            return 127
        fi
        (
            cd "$repo_root"
            "$bb_bin" "$@"
        )
        return
    fi

    if [[ -x "$repo_root/bb" ]]; then
        (
            cd "$repo_root"
            "$repo_root/bb" "$@"
        )
        return
    fi

    if command -v bb >/dev/null 2>&1; then
        (
            cd "$repo_root"
            bb "$@"
        )
        return
    fi

    if command -v go >/dev/null 2>&1; then
        (
            cd "$repo_root"
            go run ./cmd/bb "$@"
        )
        return
    fi

    echo "error: unable to find bb binary; set BB_BIN or install Go." >&2
    return 127
}
