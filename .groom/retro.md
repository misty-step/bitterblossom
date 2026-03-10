# Retro

## 2026-03-10 [issue #98](https://github.com/misty-step/bitterblossom/issues/98)

- predicted: M
- actual: M
- scope: tightened the existing conductor CLI surfaces instead of adding a new dashboard so run inspection now exposes heartbeat age, blocker context, and a single-run JSON view
- blocker: none
- pattern: when operator visibility is missing, prefer composing one deeper CLI surface from persisted run and event state before inventing another storage or UI layer

## 2026-03-07 [issue #499](https://github.com/misty-step/bitterblossom/issues/499)

- predicted: M
- actual: M
- scope: paired the ADR with the storage foundation so review governance has both policy and substrate in one reviewable slice
- blocker: none
- pattern: make governance append-only first, then move merge policy onto that ledger in follow-up issues
