# Lifecycle Status Read Path

Date: 2026-07-02
Backlog: 062 child 4

## Decision

No new status-surface join is added in this slice. The current operator read
path is enough for report-only lifecycle reflexes:

1. `bb status --json` for task/run health, parked tasks, DLQs, leases, progress,
   attention-debt, notification outbox, ingress, and gate policy.
2. `bb runs show <run-id> --json` for the lifecycle run that produced a
   recommendation.
3. `bb artifacts read <run-id> REPORT.json --json` for the lifecycle
   recommendation itself.
4. `bb gate --change <key> --json` when the recommendation references an active
   submission gate.

The lifecycle orchestrator is report-only per
`docs/lifecycle-orchestrator-authority.md`, so recommendations are intentionally
attached to the run artifact that made them. Aggregating every recommendation
into `bb status` would turn status into another workflow planner before there
is enough production run history to justify the surface.

## Evidence

Current `bb status --json` exposes:

- per-task `parked`;
- per-task `dlq.open`, `dlq.acknowledged`, and `dlq.latest_open`;
- per-task queue counts and oldest pending age;
- running progress classifications;
- active host leases;
- safe next actions for unpark, DLQ replay, recovery, inspection, or monitor;
- `guards.gate` policy;
- `guards.attention_debt` with parked task, open DLQ, awaiting recovery, stale
  run, and notification counts;
- notification outbox counts and recent failed/pending notifications.

Regression coverage already exists in:

- `tests/status_view.rs`
- `tests/status_cli.rs`

## Revisit Criteria

Add a new status join only after at least one real lifecycle orchestrator run
produces a `REPORT.json` recommendation and an operator has to manually stitch
more than these three IDs together:

- source lifecycle run id;
- recommended follow-up run id;
- submission/change key.

Until then, the artifact is the recommendation surface and status remains the
operator truth surface for plane health.
