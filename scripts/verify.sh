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

echo "==> spine LOC budget (<= 5000; the Python conductor died of bloat)"
loc=$(find src -name '*.rs' -exec cat {} + | grep -vc '^\s*$')
echo "    src LOC: $loc"
[ "$loc" -le 5000 ] || { echo "spine over the 5k LOC budget"; exit 1; }

echo "==> verify: all gates green"
