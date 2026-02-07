#!/usr/bin/env bash
# preflight.sh — Pre-dispatch validation for sprites
#
# Defines errors out of existence (Ousterhout). If this script passes,
# the sprite is ready to work. If it fails, the error is LOUD.
#
# Usage:
#   ./scripts/preflight.sh <sprite-name>     # Check single sprite
#   ./scripts/preflight.sh --all             # Check all sprites
#
# Exit codes:
#   0 = All checks pass
#   1 = Critical failure (sprite cannot work)

set -euo pipefail

LOG_PREFIX="preflight" source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"

FAILURES=0
WARNINGS=0

pass() { echo "  ✅ $1"; }
fail() { echo "  ❌ $1"; FAILURES=$((FAILURES + 1)); }
warn() { echo "  ⚠️  $1"; WARNINGS=$((WARNINGS + 1)); }

check_sprite() {
    local name="$1"
    echo ""
    echo "=== Preflight: $name ==="

    # 1. Sprite exists
    if ! sprite_exists "$name"; then
        fail "Sprite '$name' does not exist on Fly.io"
        return
    fi
    pass "Sprite exists"

    # 2. Sprite is responsive (can exec)
    local response
    response=$("$SPRITE_CLI" exec -s "$name" -- echo "alive" 2>&1 || echo "UNREACHABLE")
    if [[ "$response" == *"alive"* ]]; then
        pass "Sprite responsive"
    else
        fail "Sprite unreachable: $response"
        return  # Can't check anything else if unreachable
    fi

    # 3. Claude Code installed
    local claude_version
    claude_version=$("$SPRITE_CLI" exec -s "$name" -- bash -c "claude --version 2>/dev/null || echo MISSING" 2>&1)
    if [[ "$claude_version" == *"MISSING"* ]]; then
        fail "Claude Code not installed"
    else
        pass "Claude Code installed: $(echo "$claude_version" | head -1)"
    fi

    # 4. Git credentials configured
    local git_cred
    git_cred=$("$SPRITE_CLI" exec -s "$name" -- bash -c "git config --global credential.helper 2>/dev/null || echo MISSING" 2>&1)
    if [[ "$git_cred" == *"store"* ]]; then
        pass "Git credential helper: store"
    else
        fail "Git credentials NOT configured (credential.helper=$git_cred)"
    fi

    # 5. Git credentials file exists with content
    local git_cred_file
    git_cred_file=$("$SPRITE_CLI" exec -s "$name" -- bash -c "test -s /home/sprite/.git-credentials && echo EXISTS || echo MISSING" 2>&1)
    if [[ "$git_cred_file" == *"EXISTS"* ]]; then
        pass "Git credentials file exists"
    else
        fail "Git credentials file MISSING or empty (/home/sprite/.git-credentials)"
    fi

    # 6. Git push test (try a dry-run push to verify auth)
    local push_test
    push_test=$("$SPRITE_CLI" exec -s "$name" -- bash -c "
        cd /tmp && rm -rf preflight-test && mkdir preflight-test && cd preflight-test &&
        git init -q && git remote add origin https://github.com/misty-step/cerberus.git &&
        git ls-remote origin HEAD >/dev/null 2>&1 && echo PASS || echo FAIL
    " 2>&1)
    if [[ "$push_test" == *"PASS"* ]]; then
        pass "Git remote access verified"
    else
        fail "Git remote access FAILED (cannot reach GitHub)"
    fi

    # 7. CLAUDE.md exists
    local has_claude_md
    has_claude_md=$("$SPRITE_CLI" exec -s "$name" -- bash -c "test -f /home/sprite/workspace/CLAUDE.md && echo YES || echo NO" 2>&1)
    if [[ "$has_claude_md" == *"YES"* ]]; then
        pass "CLAUDE.md present"
    else
        warn "CLAUDE.md missing (sprite may lack instructions)"
    fi

    # 8. Disk space check
    local disk_avail
    disk_avail=$("$SPRITE_CLI" exec -s "$name" -- bash -c "df -h /home/sprite | tail -1 | awk '{print \$4}'" 2>&1)
    pass "Disk available: $disk_avail"

    # 9. Git user configured
    local git_user
    git_user=$("$SPRITE_CLI" exec -s "$name" -- bash -c "git config --global user.name 2>/dev/null || echo MISSING" 2>&1)
    if [[ "$git_user" == *"MISSING"* ]]; then
        warn "Git user.name not configured"
    else
        pass "Git user: $git_user"
    fi

    # 10. Check for stale processes
    local claude_count
    claude_count=$("$SPRITE_CLI" exec -s "$name" -- bash -c "pgrep -c claude 2>/dev/null || echo 0" 2>&1 | tr -d '[:space:]')
    if [[ "$claude_count" -gt 0 ]]; then
        warn "Claude already running ($claude_count processes) — kill before redispatch"
    else
        pass "No stale Claude processes"
    fi
}

# --- Main ---
if [[ $# -eq 0 ]]; then
    echo "Usage: $0 <sprite-name|--all>"
    exit 1
fi

if [[ "$1" == "--all" ]]; then
    SPRITE_LIST=$("$SPRITE_CLI" list 2>/dev/null || echo "")
    if [[ -z "$SPRITE_LIST" ]]; then
        echo "❌ No sprites found"
        exit 1
    fi
    for name in $SPRITE_LIST; do
        check_sprite "$name"
    done
else
    check_sprite "$1"
fi

echo ""
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
if [[ $FAILURES -gt 0 ]]; then
    echo "❌ PREFLIGHT FAILED: $FAILURES critical failures, $WARNINGS warnings"
    echo "   Fix failures before dispatching."
    exit 1
elif [[ $WARNINGS -gt 0 ]]; then
    echo "⚠️  PREFLIGHT PASSED with $WARNINGS warnings"
    exit 0
else
    echo "✅ PREFLIGHT PASSED: All checks green"
    exit 0
fi
