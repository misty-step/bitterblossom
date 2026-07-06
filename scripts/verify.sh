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

# bitterblossom-124: coverage ratchet, not an arbitrary one-time threshold --
# see docs/coverage-ratchet.md for the full policy. Baseline recorded
# 2026-07-06: 81.06% total line coverage. Floor sits a little under that so
# routine measurement noise across platforms cannot false-fail; raise the
# floor only after a deliberate coverage improvement, in the same commit,
# with a comment recording the new baseline (same convention as
# SPINE_LOC_CAP below). cargo-llvm-cov reruns the suite under instrumentation
# (mechanism cost, not workload judgment) and emits an lcov report as the
# coverage-proxy artifact this card's acceptance requires.
COVERAGE_LINE_FLOOR=80.5
echo "==> coverage ratchet (line floor <= $COVERAGE_LINE_FLOOR%; docs/coverage-ratchet.md)"
mkdir -p target/coverage
if cargo llvm-cov --fail-under-lines "$COVERAGE_LINE_FLOOR" --lcov --output-path target/coverage/lcov.info; then
  :
elif [ -n "${BB_COVERAGE_WAIVER:-}" ]; then
  echo "COVERAGE WAIVED: $BB_COVERAGE_WAIVER"
  echo "(this bypass must be visible in the PR/workflow diff that set BB_COVERAGE_WAIVER -- see docs/coverage-ratchet.md)"
else
  echo "line coverage dropped below the ratchet floor ($COVERAGE_LINE_FLOOR%)."
  echo "Fix the regression, or for a deliberate reviewed exception set BB_COVERAGE_WAIVER=\"<reason>\"."
  echo "See docs/coverage-ratchet.md."
  exit 1
fi

# bitterblossom-124: application-floor browser execution gate for the
# operator dashboard, distinct from bitterblossom-119's rendered-screenshot
# proof (states/layout) -- this is automated syntax validation, a headless
# load with zero console errors, and one real click path (auth, then a view
# switch), at desktop and mobile widths. Skips gracefully if Node/Playwright
# are not present locally (not a repo dependency; see docs/coverage-ratchet.md
# for setup); CI always installs both so the check is never silently absent
# there (.github/workflows/ci.yml).
echo "==> dashboard browser smoke (docs/coverage-ratchet.md)"
if command -v node >/dev/null 2>&1 && node -e "require.resolve('playwright')" >/dev/null 2>&1; then
  node scripts/dashboard-smoke.mjs "$PWD/target/debug/bb"
elif [ -n "${CI:-}" ]; then
  echo "CI must have Node + Playwright installed for this gate; see .github/workflows/ci.yml"
  exit 1
else
  echo "node/playwright not found locally -- skipping (required in CI). See docs/coverage-ratchet.md to install."
fi

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
cargo run --quiet -- --config examples/powder-ready-plane check
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
#
# Raised again 2026-07-05 for bitterblossom-921: reap.rs (lane-checkout
# lifecycle janitor) plus the runs.checkout_path column, `bb runs
# set-checkout-path`, and `bb janitor sweep`. Mechanism, not judgment: a
# deterministic clean-tree + fully-pushed + idle-grace check, mirroring the
# existing recovery.rs boot-classification pattern -- it decides nothing
# about whether the work was good, only whether the checkout can vanish
# with nothing lost.
#
# Raised again 2026-07-05 for bitterblossom-123: `bb doctor` (src/doctor.rs)
# is the application-floor verified-live onboarding gate -- config load,
# ledger reachability/schema, preflight::run_all (new all-tasks entry point)
# folded into one report, plus a best-effort/--expect-serve curl probe of
# the two unauthenticated routes bb serve already exposes (/health, /).
# CLI/ledger/serve-probe mechanism composing existing read paths, not
# workload judgment; no HTTP client dependency added (shells to curl like
# canary::deliver). Both raises landed independently and are additive.
# bitterblossom-930: the HITL ask/answer runtime primitive -- an `asks`
# ledger table + state machine, the `parked_on_ask` run state and its
# dispatch.rs outcome (a new terminal AttemptOutcome, no change to existing
# retry/success/failure branches), three serve.rs HTTP routes (raise, poll,
# answer -- answer reuses the existing ledger.ingest + dispatch_loop pickup
# path, the same mechanism `dlq replay` already uses for lineage-linked
# re-dispatch; no new concurrency model, no long-poll on the single-threaded
# http_loop), and a new src/ask.rs CLI module (shells to curl, no HTTP
# client dependency added, matching canary.rs/notify.rs). No module
# ballooned (main.rs/ledger.rs/serve.rs each grew ~60-90 lines; ask.rs is
# new); this is dispatch/ledger/CLI mechanism, not workload judgment.
SPINE_LOC_CAP=14100
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
