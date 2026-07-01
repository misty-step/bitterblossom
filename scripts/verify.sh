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
cargo run --quiet -- --config examples/local-plane check

echo "==> local-plane zero-credential golden path (no secrets, no network)"
BB=./target/debug/bb
CFG=examples/local-plane
$BB --config $CFG preflight hello --json >/dev/null
# invalid payload must not create a run row; before/after run count equal.
before=$($BB --config $CFG runs list --json | grep -c '"id"' || true)
$BB --config $CFG run hello --payload 'not json' >/dev/null 2>&1 || true
after=$($BB --config $CFG runs list --json | grep -c '"id"' || true)
[ "$before" = "$after" ] || { echo "local-plane smoke: invalid payload created a run ($before -> $after)"; exit 1; }
$BB --config $CFG run hello --payload '{"ok":true}' --json >/dev/null
$BB --config $CFG status --json >/dev/null

echo "==> operations smoke drill"
BB_BIN=./target/debug/bb scripts/production-ops-drill.sh --local >/dev/null

echo "==> self-drill chaos fixture"
self_drill_tmp=$(mktemp -d "${TMPDIR:-/tmp}/bb-self-drill-verify.XXXXXX")
BB_BIN=$BB scripts/self-drill-chaos.sh --report "$self_drill_tmp/REPORT.json" >/dev/null
python3 - "$self_drill_tmp/REPORT.json" <<'PY'
import json
import pathlib
import sys

report = json.loads(pathlib.Path(sys.argv[1]).read_text())
if report.get("status") != "pass":
    raise SystemExit(f"self-drill failed: {report}")
PY
rm -rf "$self_drill_tmp"

echo "==> spine LOC bloat tripwire (<= 7900; mechanism only — the Python conductor died of bloat)"
loc=$(find src -name '*.rs' -exec cat {} + | grep -vc '^\s*$')
echo "    src LOC: $loc"
if [ "$loc" -gt 7900 ]; then
  echo "spine tripped the bloat tripwire. The number is arbitrary; the invariant is not:"
  echo "src/ is MECHANISM (config, ledger, dispatch, ingress, CLI, recovery, serve, mcp), not WORKLOAD"
  echo "judgment. Ask whether what you added is mechanism — if not, move it to tasks/ +"
  echo "lane cards (that shrinks the spine). If it IS mechanism and src/ is verifiably"
  echo "lean, raising the cap is the sanctioned response, not cheating. Never golf code to"
  echo "fit, or invent an extraction because you are near the cap. Per-module sizes:"
  for f in src/*.rs; do printf '    %5d  %s\n' "$(grep -vc '^\s*$' "$f")" "$f"; done | sort -rn
  exit 1
fi

echo "==> verify: all gates green"
