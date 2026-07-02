# Lifecycle Orchestrator Authority

Date: 2026-07-02
Backlog: 062 child 3

## Decision

The lifecycle orchestrator is report-only for now. It may read the lifecycle
event payload and plane state, then write a deterministic run plan with exact
`bb run ... --payload-file ...` commands. It must not create runs, call the
ingress API, hold a plane mutation token, park or unpark tasks, resolve runs,
acknowledge DLQs, send notifications, merge PRs, or deploy.

## Why

The SDLC reflex pack exists to make follow-up work obvious and repeatable, not
to create an unbounded self-fanning loop. The deploy/prod verifier, CI
diagnoser, canary triager, and gate-blocked fix-prompt generator are already
report producers. Giving an orchestrator mutation authority before those report
contracts have enough run history would move the safety boundary from the plane
to a model prompt.

Report-only orchestration still closes the operator loop:

- the run ledger records the event that caused the orchestration run;
- the orchestrator report names the exact next commands and payload files;
- a human, supervised lane, or future narrow executor can run those commands;
- every subsequent run still enters through normal `bb run` admission, budget,
  lease, artifact, and notification paths.

## Allowed Report Shape

The orchestrator's `REPORT.json` may contain:

- `schema_version`: `bb.lifecycle_orchestrator_report.v1`;
- `event`: copied from `EVENT.json`;
- `bb_run_id`: copied from `RUN.json`;
- `plane_snapshot`: summarized status, gate, parked-task, DLQ, and recent-run
  evidence;
- `recommended_runs`: ordered list of exact commands, payload files, expected
  task names, and idempotency keys;
- `stop_conditions`: reasons not to run the plan;
- `residual_risk`: what remains unknown.

## Red Lines

- No direct writes to the ledger or production plane config.
- No generated commands that bypass `bb run`.
- No command without an idempotency key when the event can redeliver.
- No fan-out count above the task's documented budget or the report's explicit
  stop condition.
- No use of `bb task unpark`, `bb runs resolve`, `bb dlq ack`, `bb notify`, or
  merge/deploy commands in generated plans.

## Revisit Criteria

A future narrow executor can be considered only after a separate backlog item
proves all of these:

- a scoped plane token that can create runs only for an allowlisted task set;
- hard max fan-out per source event;
- idempotency-key derivation in deterministic code, not prose;
- budget preflight before run creation;
- dry-run mode with a golden fixture;
- outbox escalation when the executor refuses or partially completes a plan;
- tests showing malformed reports cannot create runs.

Until then, lifecycle orchestration is a plan artifact, not a mutation path.
