# Self-Drill Chaos Reflex Fixture

## Goal

Run the checked-in chaos drill from an isolated temporary dev plane. Deliberately
seed a stale submission-storm member and prove the stale-arm path escalates.

## Oracle

The drill calls `bb gate`, observes the notification outbox, and proves the
stale member cannot silently block a submission forever.

## Boundaries

Do not touch the production ledger directly. Do not create branches, PRs,
issues, or external notifications outside the temporary plane.

## Output

Write `REPORT.json` with the seeded submission, stale member, outbox row, and
commands run.

## Receipt

The final answer repeats the `REPORT.json` result and names the temporary plane
path used for the drill.
