# Lifecycle orchestrator run-plan fixture

## Goal

Turn one lifecycle event plus current plane state into a deterministic run
plan. Read `RUN.json` first, then read `EVENT.json`, then read plane state
through the read-only surfaces (`bb status --json`, `bb gate --json`,
`bb runs list --json`, `bb dlq list --json`). The report names the exact
follow-up `bb run ... --payload-file ... --json` commands an operator or a
narrow executor should run next. This task is a planner, not an executor.

Authority contract: `docs/lifecycle-orchestrator-authority.md`.

## Input

`EVENT.json` is any lifecycle event already handled by this plane, for example:

- `check_suite.failed` (feeds `ci-diagnose`);
- `gate.blocked` (feeds `fix-prompt`);
- `deploy_smoke.failed` or a production incident (feeds `deploy-prod-verify`);
- a Canary incident (feeds `canary-triage`).

## Oracle

`REPORT.json` includes `"schema_version": "bb.lifecycle_orchestrator_report.v1"`,
`"event"` copied from `EVENT.json`, `"bb_run_id"` copied from `RUN.json`,
`"plane_snapshot"` summarizing status, gate, parked tasks, DLQ, and recent runs,
`"recommended_runs"`, `"stop_conditions"`, `"residual_risk"`,
`"artifact_paths": ["REPORT.json"]`, and `"no_side_effects": true`.

Every entry in `"recommended_runs"` is an ordered plan step with `"command"`
(an exact `bb run ... --payload-file ... --json` invocation), `"task"` (the
expected task name), `"payload_file"`, and `"idempotency_key"`. No recommended
command omits its idempotency key when the source event can redeliver.

## Boundaries

This task is `report_only`. It writes `REPORT.json` only.

- No code edits.
- No branches.
- No PRs.
- No merges.
- No deploys.
- No comments.
- No task parking or unparking.
- No run resolution.
- No DLQ acknowledgement or replay.
- No notification mutation.
- No direct ledger writes.
- No generated command that bypasses `bb run`.
- No recommended `bb task unpark`, `bb runs resolve`, `bb dlq ack`, `bb notify`,
  merge, or deploy command inside the plan.
- No fan-out above this task's documented budget or the report's own stop
  conditions.

## Output

Write `REPORT.json` with the ordered run plan, stop conditions, and residual
uncertainty. Do not execute any recommended command.

## Receipt

The final answer repeats the ordered `bb run` commands and their idempotency
keys, and names the stop conditions that would cancel the plan.
