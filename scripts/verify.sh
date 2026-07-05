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

echo "==> vendored roster CLI"
roster_target="$PWD/target/vendor-roster"
CARGO_TARGET_DIR="$roster_target" cargo build --quiet --manifest-path vendor/roster/Cargo.toml --bin roster
PATH="$roster_target/debug:$PATH"
export PATH

echo "==> product/instance split guard"
if grep -Eq '^[[:space:]]*COPY[[:space:]]+plane([[:space:]]|$)' Dockerfile; then
  echo "Dockerfile must not COPY production plane/ into the product image"
  exit 1
fi
grep -qx 'plane' .dockerignore || {
  echo ".dockerignore must exclude plane/ so remote image builds do not upload instance config"
  exit 1
}
if git ls-files 'plane/**' | grep -q .; then
  echo "production plane/ instance config must not be tracked in the product repo"
  git ls-files 'plane/**'
  exit 1
fi
if git ls-files --error-unmatch canary-services.toml >/dev/null 2>&1; then
  echo "canary-services.toml is stale instance topology and must not be tracked"
  exit 1
fi

echo "==> plane configs validate (bb check)"
cargo run --quiet -- --config examples/demo-plane check
cargo run --quiet -- --config examples/local-plane check
cargo run --quiet -- --config examples/canary-responder-plane check
cargo run --quiet -- --config examples/review-factory-plane check
cargo run --quiet -- --config examples/roster-cerberus-plane check
cargo run --quiet -- --config examples/docs-sync-plane check
cargo run --quiet -- --config examples/ci-audit-plane check
cargo run --quiet -- --config examples/hygiene-plane check
cargo run --quiet -- --config examples/moments-plane check
cargo run --quiet -- --config tests/fixtures/public-plane check

echo "==> local-plane zero-credential golden path (no secrets, no network)"
BB=./target/debug/bb
local_plane_tmp=$(mktemp -d "${TMPDIR:-/tmp}/bb-local-plane-verify.XXXXXX")
cp -R examples/local-plane/. "$local_plane_tmp/"
rm -rf "$local_plane_tmp/.bb"
CFG=$local_plane_tmp
run_count() {
  $BB --config $CFG runs list --json | python3 -c 'import json, sys; print(len(json.load(sys.stdin)))'
}
$BB --config $CFG preflight hello --json >/dev/null
# invalid payload must not create a run row; before/after run count equal.
before=$(run_count)
$BB --config $CFG run hello --payload 'not json' >/dev/null 2>&1 || true
after=$(run_count)
[ "$before" = "$after" ] || { echo "local-plane smoke: invalid payload created a run ($before -> $after)"; exit 1; }
run_json=$($BB --config $CFG run hello --payload '{"ok":true}' --json)
run_id=$(printf '%s' "$run_json" | python3 -c 'import json, sys; print(json.load(sys.stdin)["run"]["id"])')
$BB --config $CFG status --json >/dev/null
$BB --config $CFG runs show "$run_id" --json >/dev/null
$BB --config $CFG artifacts read "$run_id" REPORT.json --json >/dev/null
rm -rf "$local_plane_tmp"

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

# Raised 2026-07-03 for lead dispatch/log-follow ergonomics:
# enqueueing a brief-backed manual run and following ledger/artifact output are
# operator-plane mechanism, not workload judgment.
# Raised 2026-07-04 for external-run registration:
# local Claude/Codex/Herdr dispatch receipts are ledger/API/dashboard mechanism,
# not workload judgment; keep route-through authority out until separately earned.
# bb-912 raised this for generic serve/ingress/budget security mechanism:
# bounded request reads, panic containment, constant-time auth, and atomic budget admission.
# bitterblossom-107 raised this to 11550 for sprite command-binary preflight
# checks (bb preflight <task> now validates command-harness binaries exist on
# the declared sprite host, not only the local plane host).
# Raised 2026-07-04 for bitterblossom-088: a required gate member (e.g. the
# Thermo-Nuclear maintainability lens) can be explicitly waived per change with
# a risk-tier-tagged reason instead of hanging the gate pending forever — the
# same generic mechanism shape as the existing finding-level rejection/arbiter
# path (member_waivers table, Ledger::waive_member/member_waiver, evaluate()
# quorum arithmetic, `bb submit waive`). Gate/ledger mechanism, not workload
# judgment: which diffs qualify for a risk tier stays driver/operator
# judgment recorded in the reason string, not plane policy.
# Raised 2026-07-04 for bitterblossom-102: bulk `bb task unpark` safety is
# operator recovery mechanism (ledger preview, scoped release, and confirmation),
# not workload judgment. The policy decision about which external targets are
# stale stays with the operator/runbook; the plane only bounds the release.
# Raised 2026-07-04 for bitterblossom-109: outbound notification delivery
# (notify.rs) now captures the actual HTTP status code and a bounded,
# truncated response snippet per attempt instead of trusting curl's exit
# code alone — a delivery retrying after a non-2xx response is now
# debuggable (last_status_code/last_response in `bb notify list`/`/api/notify`)
# without ever persisting an unbounded or secret-bearing response body.
# Ledger/delivery mechanism, not workload judgment.
# Raised 2026-07-04 for bitterblossom-101: `bb artifacts bundle` exports a
# portable manifest directory from already-recorded attempt artifacts. It is
# artifact transport/containment mechanism (attempt identity, relative paths,
# binary/oversized/symlink policy), not workload judgment about artifact
# meaning.
# Raised 2026-07-05 for bitterblossom-925: an agent can declare
# optional_secrets alongside secrets -- unresolvable ones degrade the run
# (absent from the workload env) instead of dead-lettering it, with a
# distinct missing_optional_secret preflight finding kind. Ledger/dispatch
# mechanism (a name is never in both lists; forbidden ANTHROPIC_API_KEY/
# OPENAI_API_KEY checks now cover both), not workload judgment about which
# secrets a given task actually needs.
# Raised 2026-07-05 for bitterblossom-915: workload_env() now passes
# OP_SERVICE_ACCOUNT_TOKEN through the local-substrate PASS allowlist like
# PATH, plus a regression test spawning a real child process to prove the
# token reaches it. Env-passthrough plumbing and its own proof, not workload
# judgment about which task needs 1Password.
# Raised 2026-07-05 for bitterblossom-116: the opt-in mutating MCP bb_dispatch
# tool plus the dispatch::build_dispatch_job_payload/default_dispatch_task/
# dispatch_idempotency_key helpers it shares with the CLI `bb dispatch`
# command (moved out of main.rs so the two entry points cannot drift).
# Ingress/dispatch mechanism gated by BB_MCP_ENABLE_DISPATCH -- the read-only
# tool table stays unconditional -- not workload judgment about what an ad
# hoc dispatch job should contain.
SPINE_LOC_CAP=12650
echo "==> spine LOC bloat tripwire (<= $SPINE_LOC_CAP; mechanism only — the Python conductor died of bloat)"
loc=$(find src -name '*.rs' -exec cat {} + | grep -vc '^\s*$')
echo "    src LOC: $loc"
if [ "$loc" -gt "$SPINE_LOC_CAP" ]; then
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
