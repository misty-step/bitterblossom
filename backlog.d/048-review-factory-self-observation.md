# Make the review factory explain its own failures before external observability

Priority: P1 · Status: pending · Estimate: L

## Goal

Give operators and future observer workloads a ledger-native health report for
review and verdict-storm runs before adding a Raindrop dependency.

## Oracle

- [ ] A CLI or API JSON surface clusters recent runs by task, agent version,
      state, failure reason, cost, duration, parked state, and DLQ status.
- [ ] The report flags the current live-plane facts: review v3 has a recent
      unparseable-output failure amid cheap successes, correctness has timeout
      failures, and `security` is parked after a cost breach.
- [ ] The output names safe operator actions, such as inspect artifact, rebind
      model, unpark after reason cleared, replay only pre-execute DLQ, or leave
      blocked.
- [ ] Tests use ledger fixtures; no live model or external observability
      service is required.

## Notes

This is the local baseline that backlog 033 should compare Raindrop against.
The 2026-06-13 groom read `bb --config plane runs list --json` and
`bb --config plane task list --json`; those commands already expose the raw
facts but force every operator or agent to rebuild the clustering logic.

Do not add review-specific branches to dispatch, substrate, or harness. The
surface should be a generic run-health view that Raindrop export, the gardener
task, or a human can consume.

## Mega groom disposition 2026-06-13

This becomes the seed for
`backlog.d/052-ledger-native-operator-truth-surface.md`. The review factory is
the first evidence source, but the resulting surface must stay generic enough
for any workload, the Bitterblossom skill, and future observer tasks.
