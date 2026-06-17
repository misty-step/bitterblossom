#!/bin/sh
# The repo gate: everything mechanical that "done" requires, one
# entrypoint, identical locally and in CI. Live-evidence recipes that a
# green gate cannot replace live in CLAUDE.md (Verification).
set -eu
cd "$(dirname "$0")/.."

echo "==> fmt"
cargo fmt --check

echo "==> clippy"
cargo clippy --all-targets -- -D warnings

echo "==> tests"
cargo test

echo "==> OpenRouter model catalog fixture"
scripts/check-model-catalog.sh --catalog tests/fixtures/openrouter-models-current.json --json >/dev/null

echo "==> plane configs validate (bb check)"
cargo run --quiet -- --config plane check
cargo run --quiet -- --config examples/demo-plane check

echo "==> operations smoke drill"
BB_BIN=./target/debug/bb scripts/production-ops-drill.sh --local >/dev/null

echo "==> spine LOC budget (<= 5300; mechanism only — the Python conductor died of bloat)"
loc=$(find src -name '*.rs' -exec cat {} + | grep -vc '^\s*$')
echo "    src LOC: $loc"
if [ "$loc" -gt 5300 ]; then
  echo "spine over the LOC budget. The cap is a proxy for one invariant: src/ is"
  echo "MECHANISM (config, ledger, dispatch, ingress, CLI, recovery), not WORKLOAD"
  echo "judgment. Before raising it, ask whether what you added is mechanism — if not,"
  echo "move it to tasks/ + lane cards (that shrinks the spine). Raise the cap only as"
  echo "a conscious re-baseline when src/ is verifiably lean. Per-module sizes:"
  for f in src/*.rs; do printf '    %5d  %s\n' "$(grep -vc '^\s*$' "$f")" "$f"; done | sort -rn
  exit 1
fi

echo "==> verify: all gates green"
