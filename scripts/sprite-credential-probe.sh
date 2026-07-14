#!/bin/sh
# Names-only credential-surface probe for a sprite host
# (bitterblossom-971; docs/credential-refusal-doctrine.md).
#
# Answers, from the sprite a lane would actually run on: which credential
# surfaces exist there at all? It reports REACHABILITY AND NAMES ONLY —
# no credential value, keychain item, or file content is ever read, printed,
# or transmitted. Safe to run against production sprites.
#
# Usage: scripts/sprite-credential-probe.sh [org/name | name]   (default: bb-builder)
#
# Interpretation (see the doctrine doc):
# - security(1)/keychain/op absent + credential-free default env = the
#   operator's admin credentials are unreachable from lanes on this sprite.
# - PRESENT baked identities (~/.config/gh, ~/.claude, ~/.codex, git
#   credential.helper) are lane-class platform credentials by design on
#   dispatch sprites — audit that set whenever a checkpoint is minted;
#   anything broader silently widens every lane on the sprite.
set -eu

SPRITE=${1:-bb-builder}
command -v sprite >/dev/null || { echo "sprite CLI not found on PATH" >&2; exit 1; }

# The sprite CLI resolves its org from cwd path history; run the relay from
# HOME and let org/name host syntax pin the org when provided (CLAUDE.md).
cd "$HOME"
case "$SPRITE" in
  */*) set -- -o "${SPRITE%%/*}" -s "${SPRITE#*/}" ;;
  *)   set -- -s "$SPRITE" ;;
esac

echo "== credential-surface probe on sprite '$SPRITE' (names only, no values) =="
sprite exec "$@" -- sh -c '
  echo "host: $(uname -s) $(hostname)"
  echo "user: $(id -un) uid=$(id -u)"
  command -v security >/dev/null 2>&1 && echo "security(1): PRESENT" || echo "security(1): absent"
  [ -d "$HOME/Library/Keychains" ] && echo "macOS keychain dir: PRESENT" || echo "macOS keychain dir: absent"
  command -v op >/dev/null 2>&1 && echo "op CLI: PRESENT" || echo "op CLI: absent"
  git config --global credential.helper >/dev/null 2>&1 && echo "git credential.helper: SET" || echo "git credential.helper: unset"
  echo "--- credential-adjacent home paths (presence only):"
  for p in .ssh .netrc .git-credentials .aws .config/gh .config/op .claude .claude.json .codex .config/powder; do
    [ -e "$HOME/$p" ] && echo "  $p: PRESENT" || echo "  $p: absent"
  done
  echo "--- env var NAMES in default sprite shell (values never printed):"
  env | cut -d= -f1 | sort | sed "s/^/  /"
'
