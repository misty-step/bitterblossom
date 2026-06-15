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

echo "==> plane configs validate (bb check)"
cargo run --quiet -- --config plane check
cargo run --quiet -- --config examples/demo-plane check
cargo run --quiet -- --config examples/canary-responder-plane check

echo "==> spine LOC budget (<= 5000; the Python conductor died of bloat)"
loc=$(find src -name '*.rs' -exec cat {} + | grep -vc '^\s*$')
echo "    src LOC: $loc"
[ "$loc" -le 5000 ] || { echo "spine over the 5k LOC budget"; exit 1; }

echo "==> verify: all gates green"
