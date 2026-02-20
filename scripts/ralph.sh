#!/usr/bin/env bash
set -euo pipefail

WS="${WORKSPACE:-$HOME/workspace/repo}"
PROMPT="$WS/.dispatch-prompt.md"
RALPH_LOG="$WS/ralph.log"
RALPH_LOG_MAX_BYTES="${RALPH_LOG_MAX_BYTES:-20971520}"   # 20 MiB
RALPH_LOG_KEEP_BYTES="${RALPH_LOG_KEEP_BYTES:-15728640}" # 15 MiB
BB_TIMEOUT_SEC="${BB_TIMEOUT_SEC:-1800}"
AUTO_COMPLETE_ON_PR="${AUTO_COMPLETE_ON_PR:-1}"

log() { printf '%s\n' "$*" | tee -a "$RALPH_LOG"; }
log_err() { printf '%s\n' "$*" | tee -a "$RALPH_LOG" >&2; }

trim_log() {
  local size tmp keep
  keep="$RALPH_LOG_KEEP_BYTES"
  if [[ "$keep" -gt "$RALPH_LOG_MAX_BYTES" ]]; then
    keep="$RALPH_LOG_MAX_BYTES"
  fi

  size="$(wc -c <"$RALPH_LOG" 2>/dev/null || echo 0)"
  size="${size//[[:space:]]/}"
  if [[ "$size" -le "$RALPH_LOG_MAX_BYTES" ]]; then
    return 0
  fi

  tmp="$RALPH_LOG.tmp"
  if ! tail -c "$keep" "$RALPH_LOG" >"$tmp" 2>/dev/null; then
    rm -f "$tmp"
    return 0
  fi
  if ! cat "$tmp" >"$RALPH_LOG"; then
    rm -f "$tmp"
    return 0
  fi
  rm -f "$tmp"
}

maybe_auto_complete_from_open_pr() {
  [[ "$AUTO_COMPLETE_ON_PR" == "1" ]] || return 1
  [[ -d "$WS/.git" ]] || return 1

  local branch dirty commits_ahead pr_url
  branch="$(cd "$WS" && git branch --show-current 2>/dev/null || true)"
  [[ -n "$branch" ]] || return 1
  [[ "$branch" != "master" && "$branch" != "main" ]] || return 1

  dirty="$(cd "$WS" && git status --porcelain 2>/dev/null | wc -l | tr -d ' ' || echo 0)"
  [[ "${dirty:-0}" == "0" ]] || return 1

  commits_ahead="$(cd "$WS" && (git rev-list --count origin/master..HEAD 2>/dev/null || git rev-list --count origin/main..HEAD 2>/dev/null || echo 0))"
  [[ "${commits_ahead:-0}" -gt 0 ]] || return 1

  pr_url="$(cd "$WS" && gh pr list --head "$branch" --state open --json url --jq '.[0].url' 2>/dev/null || true)"
  [[ -n "$pr_url" && "$pr_url" != "null" ]] || return 1

  cat > "$WS/TASK_COMPLETE" <<EOF
Auto-complete fallback
Branch: $branch
PR: $pr_url
Commits ahead: $commits_ahead
Generated: $(date -Iseconds)
EOF

  log "[ralph] auto-complete fallback wrote TASK_COMPLETE from open PR: $pr_url"
  return 0
}

mkdir -p "$WS"
touch "$RALPH_LOG"

[ ! -f "$PROMPT" ] && log_err "[ralph] no prompt file at $PROMPT" && exit 1

[ -f "$WS/TASK_COMPLETE" ] && log "[ralph] TASK_COMPLETE already present" && exit 0
[ -f "$WS/TASK_COMPLETE.md" ] && log "[ralph] TASK_COMPLETE.md already present" && exit 0
[ -f "$WS/BLOCKED.md" ] && log_err "[ralph] BLOCKED pre-run: $(head -3 "$WS/BLOCKED.md")" && exit 2

log "[ralph] harness=claude model=sonnet-4.6 mode=plugin"

set +e
timeout "$BB_TIMEOUT_SEC" \
  claude -p --dangerously-skip-permissions --permission-mode bypassPermissions --verbose --output-format stream-json \
  < "$PROMPT" 2>&1 | grep --line-buffered -F -v '"type":"system","subtype":"init"' | tee -a "$RALPH_LOG"
claude_rc="${PIPESTATUS[0]}"
set -e

trim_log

if [[ "${claude_rc:-0}" == "124" ]]; then
  log_err "[ralph] claude run timed out after ${BB_TIMEOUT_SEC}s"
  exit 1
fi

[ -f "$WS/TASK_COMPLETE" ] && log "[ralph] TASK_COMPLETE found" && exit 0
[ -f "$WS/TASK_COMPLETE.md" ] && log "[ralph] TASK_COMPLETE.md found" && exit 0
[ -f "$WS/BLOCKED.md" ] && log_err "[ralph] BLOCKED: $(head -3 "$WS/BLOCKED.md")" && exit 2

if maybe_auto_complete_from_open_pr; then
  exit 0
fi

if [[ "${claude_rc:-0}" != "0" ]]; then
  log_err "[ralph] claude exited ${claude_rc}"
  exit "${claude_rc}"
fi

log_err "[ralph] no TASK_COMPLETE/BLOCKED signal after successful claude run"
exit 1
