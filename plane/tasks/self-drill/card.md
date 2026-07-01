# Self-Drill Chaos Reflex

Run the checked-in chaos drill exactly as configured by the command harness.
The drill must:

- create only an isolated temporary dev plane;
- deliberately seed a stale submission-storm member;
- call `bb gate` through the public CLI;
- prove `submission_escalated` reached the durable notification outbox and the
  notify transport stub;
- write `REPORT.json` with status, gate decision, member status, outbox status,
  and residual errors.

Do not touch the production ledger directly. Do not create branches, PRs,
issues, or external notifications outside the temporary plane.
