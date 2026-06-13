# Build a ledger-native operator truth surface

Priority: P1 | Status: ready | Estimate: L

## Goal

Give humans and agents a generic, decision-ready health view over tasks, runs,
cost, failures, parked state, queue pressure, and dead letters without
workload-specific logic.

## Oracle

- [ ] A CLI and/or API JSON surface clusters recent runs by task, agent
      version, state, reason, cost, duration, parked state, DLQ status, and
      queue age.
- [ ] The surface names safe next actions: inspect artifact, unpark only after
      reason cleared, replay pre-execute DLQ, resolve awaiting recovery, or
      leave blocked.
- [ ] Fixtures cover review/verdict-storm failures, parked tasks, cheap
      successes, expensive failures, and DLQ rows.
- [ ] No review-specific branch lands in dispatch, substrate, harness, or
      ledger.
- [ ] The output can be consumed by the Bitterblossom skill and by future
      observer workloads.

## Children

1. Define the generic health JSON shape.
2. Implement the ledger query and CLI/API exposure.
3. Add examples to `skills/bitterblossom/references/operator-recipes.md`.
4. Use the surface as the baseline comparison for backlog 033.

## Notes

Why: 048 identified the missing local baseline before external observability.
The product and ops lanes reframed it as a product feature: operators should
not have to synthesize raw run rows under pressure.

Evidence:

- `project.md:104-111` says cost, budget burn, retries, dead letters, queue
  pressure, and telemetry must be visible from the CLI.
- Current `bb runs list --json` and `bb task list --json` expose facts but not
  grouped diagnosis or safe next actions.
