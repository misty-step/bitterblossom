# Self-Drill Chaos Reflex Fixture

Run the checked-in chaos drill from an isolated temporary dev plane. Deliberately
seed a stale submission-storm member, call `bb gate`, prove escalation reached
the notification outbox, and write `REPORT.json`.

Do not touch the production ledger directly. Do not create branches, PRs,
issues, or external notifications outside the temporary plane.

