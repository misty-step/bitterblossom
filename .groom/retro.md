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
- scope: tightened the existing run surfaces instead of adding a new operator path, and promoted blocking reasons into stable JSON output
- blocker: none
- pattern: when observability already has durable state, ship the operator contract by shaping one serializer and testing against it
