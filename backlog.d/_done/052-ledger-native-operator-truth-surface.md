# Build a ledger-native operator truth surface

Priority: P1 | Status: done | Estimate: L

## Goal

Give humans and agents a generic, decision-ready health view over tasks, runs,
cost, failures, parked state, queue pressure, and dead letters without
workload-specific logic.

## Oracle

- [x] A CLI and/or API JSON surface clusters recent runs by task, agent
      version, state, reason, cost, duration, parked state, DLQ status, and
      queue age.
- [x] The surface names safe next actions: inspect artifact, unpark only after
      reason cleared, replay pre-execute DLQ, resolve awaiting recovery, or
      leave blocked.
- [x] Fixtures cover review/verdict-storm failures, parked tasks, cheap
      successes, expensive failures, and DLQ rows.
- [x] No review-specific branch lands in dispatch, substrate, harness, or
      ledger.
- [x] The output can be consumed by the Bitterblossom skill and by future
      observer workloads.

## Children

1. Define the generic health JSON shape.
2. Implement the ledger query and CLI/API exposure.
3. Add examples to `skills/bitterblossom/references/operator-recipes.md`.
4. Use the surface as the baseline comparison for backlog 033.

## Delivery 2026-06-13

- Added `bb status [--json]` and `GET /api/status`, both backed by
  `src/health.rs`.
- Status JSON groups each task's recent runs by state and includes agent
  binding, cost, duration, latest reason, parked state, queue counts,
  oldest pending age, DLQ counts, and safe next actions.
- Updated `skills/bitterblossom/`, `skills/bitterblossom-dogfood/`,
  operator recipes, README, and spine command docs to advertise the surface.
- Moved ledger and harness parser fixtures from in-source unit tests to
  integration tests, preserving coverage while keeping the spine under the
  5k LOC budget.

Evidence:

- Focused tests: `cargo test --test status_cli --test status_view
  --test skill_artifacts`.
- Repo gate: `./scripts/verify.sh` passed with `src LOC: 4992`.
- Live dogfood: `./target/debug/bb --config plane status` surfaced
  `parked=1`, `open_dlq=2`, `security` action
  `unpark_after_reason_cleared`, and product/verify actions
  `replay_pre_execute_dlq`.

## Notes

Why: 048 identified the missing local baseline before external observability.
The product and ops lanes reframed it as a product feature: operators should
not have to synthesize raw run rows under pressure.

Evidence:

- `project.md:104-111` says cost, budget burn, retries, dead letters, queue
  pressure, and telemetry must be visible from the CLI.
- Current `bb runs list --json` and `bb task list --json` expose facts but not
  grouped diagnosis or safe next actions.
