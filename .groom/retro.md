# Retro

## 2026-03-07 [issue #499](https://github.com/misty-step/bitterblossom/issues/499)

- predicted: M
- actual: M
- scope: paired the ADR with the storage foundation so review governance has both policy and substrate in one reviewable slice
- blocker: none
- pattern: make governance append-only first, then move merge policy onto that ledger in follow-up issues

## 2026-03-09 [issue #98](https://github.com/misty-step/bitterblossom/issues/98)

- predicted: M
- actual: M
- scope: expanded the operator surface beyond raw event lines so run inspection now returns a stable run envelope plus recent event context
- blocker: none
- pattern: when a CLI surface becomes the operator contract, encode recovery fields in the surface itself instead of expecting callers to join SQLite tables manually
