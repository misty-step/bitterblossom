#!/bin/sh
# The repo gate: everything mechanical that "done" requires, one
# entrypoint, identical locally and in CI. Live-evidence recipes that a
# green gate cannot replace live in CLAUDE.md (Verification).
set -eu
cd "$(dirname "$0")/.."

# bitterblossom-974: the same secret scan CI enforces on every branch
# (.github/workflows/secret-scan.yml), bound into the local gate so agents
# hit it before pushing. Repo-owned rules: .gitleaks.toml; acknowledged
# historical fingerprints: .gitleaksignore. The selftest plants synthetic
# fixtures (never committed) and proves every custom rule still fires --
# the old TruffleHog gate rotted invisibly; this one cannot.
echo "==> secret scan (gitleaks; .gitleaks.toml)"
if command -v gitleaks >/dev/null 2>&1; then
  scripts/secret-scan-selftest.sh
  gitleaks git --redact --no-banner .
elif [ -n "${CI:-}" ]; then
  echo "CI must have gitleaks installed for this gate; see .github/workflows/ci.yml"
  exit 1
else
  echo "gitleaks not found locally -- skipping (enforced in CI by .github/workflows/secret-scan.yml). Install: brew install gitleaks"
fi

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
#
# bitterblossom-930: raised 80.5 -> 84.0 after fixing the real cause of a
# false coverage regression, not papering over it. tests/serve.rs's
# ChildGuard SIGKILLed every spawned `bb serve` test process; instrumented
# coverage only flushes .profraw on a normal exit (an unhandled SIGTERM
# behaved identically to SIGKILL -- verified empirically), so all ~24 tests'
# worth of already-executing HTTP-route coverage in serve.rs was invisible
# to this measurement, not just untested. Fix: `bb serve` now installs a
# SIGTERM handler that returns from main normally (src/serve.rs
# `install_graceful_shutdown_handler`), and ChildGuard sends SIGTERM first
# (falling back to SIGKILL only if the process hangs). New baseline:
# 84.81% total line coverage, serve.rs alone 34.18% -> 79.19%. Floor sits a
# little under that for the same measurement-noise reason as the original
# baseline.
COVERAGE_LINE_FLOOR=84.0
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
cargo run --quiet -- --config examples/powder-chew-dev-plane check
cargo run --quiet -- --config examples/estate-execution-plane check
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
# bitterblossom-933: 14100 -> 14450 for a new src/glass.rs (215 lines) -- the
# run plane's own lifecycle emitter (dispatched/completed/asked/resumed
# posts), shelling to curl exactly like canary.rs/notify.rs/ask.rs, plus a
# handful of call sites at existing dispatch.rs/serve.rs lifecycle seams and
# a small ledger addition (`glass_session_id`, mirroring `ask_token`) for
# glass's own real session-id contract, proven live against the deployed
# instance: an unrecognized session_id 404s, so bb persists whatever id
# glass assigns on the first post and reuses it, rather than inventing one.
# No module ballooned. This is the observability floor the card asks for --
# "a small emitter at existing lifecycle seams, not a new subsystem" -- not
# workload judgment.
# bitterblossom-935: 14450 -> 14500 for the opencode harness backend in
# harness.rs (build_command arm + parse_opencode/partial_opencode_stats,
# ~95 lines) -- a fifth harness class alongside claude/codex/pi/omp in the
# exact same file and shape as the existing four (build a command, parse
# final JSONL output, parse partial/streaming stats), no new module, no
# new subsystem. This is dispatch mechanism, not workload judgment.
# bitterblossom-934: 14500 -> 14600 for agents_view()/roster_provenance_view()
# in serve.rs (~65 lines, same shape as the existing tasks_view() a few
# lines above it: a read-only JSON projection over already-loaded plane
# config for a new dashboard API route) plus a declarative `archived: bool`
# field on TaskSpec in spec.rs (~10 lines, mirrors existing optional task
# fields like `verdict`). No new module, no new subsystem, no workload
# judgment -- the plane still holds none: archived is an operator-declared
# flag the dashboard reads, not a decision bb makes.
# bitterblossom-938: 14600 -> 14900 for src/substrate/tailnet.rs (~300
# non-blank lines) -- a third Substrate/Session implementation alongside
# local.rs/sprites.rs, same shape as sprites.rs (remote exec, workspace
# dir, marker/pidfile probe) but ssh transport instead of the Fly-specific
# sprite CLI, registered through the existing `for_task()` factory with no
# new abstraction. Edge-case policy for an unreachable host reuses the
# plane's existing dead-letter/replay mechanism rather than inventing a
# new parking state (see docs/spine.md "Substrate contract"). This is
# dispatch mechanism, not workload judgment.
# bitterblossom-956: 14900 -> 15100 for glass visibility of external
# (register-through) runs -- the exact same observability floor bb-933
# already gives dispatched runs, extended to the interactive/local sessions
# that register via POST /api/external-runs. src/glass.rs gains
# post_external_registered/post_external_completed plus a shared deliver_post
# helper the existing publish() now also routes through (net emitter growth
# ~85 lines); ledger.rs gains an external_runs.glass_session_id column and
# two get/set helpers mirroring the runs.glass_session_id pair (~25 lines);
# serve.rs wires the two emitter calls into the existing external-run
# POST/PATCH handlers (~15 lines). No new module, no new subsystem, no
# workload judgment -- observability of a run the plane already records,
# not a decision the plane makes. See docs/spine.md "Glass lifecycle emitter".
# bitterblossom-960: 15100 -> 15200 for cost governor slice 1 -- a
# repo-scoped daily cost ceiling (`WorkloadRepoSpec.max_cost_per_day_usd`,
# a sibling field to the existing `budget_caps`) that contains an
# overspending repo's blast radius to that repo alone, instead of the
# plane-global ceiling's today blast radius across every task on the
# plane. spec.rs gains the field plus a non-negative validation check
# (~15 lines); ledger.rs gains `cost_today_for_repo` and
# `budget_events_today_count` (~28 lines), the latter mirroring the
# existing `cost_today`/`runs_today` query shape; budget.rs gains the
# repo-ceiling check plus a `repo_daily_ceiling` lookup helper that derives
# the repo namespace from the task name's existing `<repo>/<short>`
# convention -- no new field on `Task` (~26 lines); dispatch.rs wraps the
# existing budget_blocked notify call in an escalate-once-per-day guard so
# repeated same-day, same-kind triggers (a parked or ceiling-blocked task
# hit by every subsequent webhook/cron redelivery) stop grinding out
# identical notifications while every blocked run still lands its own
# ledger row (~9 lines). No new module, no new subsystem, no workload
# judgment -- this is the same class of budget-governance arithmetic as
# the existing global daily ceiling it sits beside.
# Linejam DigitalOcean cutover: 15200 -> 15500 for durable attempt-artifact
# snapshots. artifacts.rs captures public top-level files at the existing
# finish_attempt seam, bounds each stored text body at the existing 1 MiB read
# limit, and projects the same list/read/bundle contracts from SQLite when an
# ephemeral runtime loses its attempt directories (~175 lines). ledger.rs adds
# the additive table access and makes snapshot failure reject success instead
# of claiming evidence durability it did not achieve (~90 lines); dispatch.rs
# maps that rejection onto the existing executed-attempt failure path (~20
# lines). No workload identity or judgment enters the spine: this is generic
# persistence and operator read-path mechanism for every task.
# bitterblossom-workflow-store: 15500 -> 17021 (exact actual, zero banked
# slack -- master sat at 15499 under a 15500 cap) for the revisioned
# workflow configuration store, the ratified 2026-07-11 product contract's
# foundation ("the database is authoritative for active configuration;
# every edit is an immutable revision"). Non-blank deltas vs master:
# src/workflow.rs +895 (new), src/serve.rs +215, src/main.rs +411,
# src/lib.rs +1. workflow.rs is config/ledger mechanism only: four tables
# (workflows, workflow_revisions, workflow_events audit, workflow_runs
# pinning), lifecycle arithmetic (draft/active/paused/archived, rollback =
# old snapshot as a NEW revision), canonical-JSON revision storage with
# TOML import/export interchange (identical import is a no-op, so files
# never become a second authority), acceptance-time revision pinning, and
# a line diff. serve.rs exposes the same store functions over
# /api/workflows*; main.rs exposes them as `bb workflow ...` -- one store,
# two projections, per the card's "CLI and HTTP create/read/diff/activate
# the same immutable revisions" oracle. No workload judgment entered the
# spine: the store validates document *structure* (names, route targets,
# trigger shapes) and never interprets goals, outcomes, or agent behavior.
# bitterblossom-estate-execution-contract: 17021 -> 17530 (exact actual,
# zero banked slack) for generic immutable checkout and artifact-retention
# mechanism. spec.rs declares and validates full Git object and lock blobs;
# substrate/mod.rs is the deep shared module: descriptor-relative no-follow
# source/destination access, bounded binary-exact remote collection, immutable
# checkout plus working-byte lock verification, and reserved evidence paths.
# All three adapters call that mechanism; no infrastructure authority or
# product judgment enters the spine.
# bitterblossom-969: 17594 -> 17649 (exact actual, zero banked slack) for
# cost-visibility mechanism: harness.rs +14 (reports_cost — which harness
# parsers actually populate cost_usd, the datum every spend control reads),
# budget.rs +41 (cost_blind_harness admission refusal: an agent whose
# declared secrets include OPENROUTER_API_KEY on a harness that cannot
# report cost is invisible to the per-run cap, both daily ceilings, and the
# in-flight monitor — refused pre-dispatch unless an effective provider-side
# child-key cap is declared; the secret name is the definitive signal, auth
# label and provider string deliberately excluded after both were executed
# as bypasses in review). Pure spend-containment mechanism; no judgment.
# bitterblossom-971: 17649 -> 17657 (exact actual, zero banked slack) for the
# credential-refusal guardrail's one mechanism seam: harness.rs gains
# `commission_prompt()`, the uniform commission preamble every dispatched
# lane receives, now carrying the refused-credential STOP-and-report rule
# (a 401/403 on a declared credential is a boundary, never a prompt to
# locate a stronger credential — the 2026-07-09 keychain-escalation
# incident). Plane security policy on the same tier as env scrubbing and
# `unset GH_TOKEN`, not workload judgment; dispatch.rs and both
# argv-prompt harness arms now share the one string instead of
# duplicating it. Doctrine: docs/credential-refusal-doctrine.md.
SPINE_LOC_CAP=17657
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
