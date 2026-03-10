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

## 2026-03-09 [issue #98](https://github.com/misty-step/bitterblossom/issues/98)

- predicted: M
- actual: M
- scope: tightened the existing run surfaces instead of adding a new operator path, and promoted blocking reasons into stable JSON output
- blocker: none
- pattern: when observability already has durable state, ship the operator contract by shaping one serializer and testing against it

## 2026-03-09 [issue #102](https://github.com/misty-step/bitterblossom/issues/102)

- predicted: L
- actual: M
- scope: proved the trace bullet with acceptance-focused run-store assertions and tightened late trusted-review handling so duplicates and low-severity nits no longer cause churn
- blocker: none
- pattern: acceptance proofs stay durable when the operator-visible commands read the same ledger states the tests assert

## 2026-03-09 [issue #468](https://github.com/misty-step/bitterblossom/issues/468)

- predicted: M
- actual: M
- scope: moved stale reclaim from silent backlog cleanup into explicit lease acquisition, added heartbeat refresh during long polling, and documented the new operator-visible reclaim events
- blocker: none
- pattern: when a control-plane recovery path matters to operators, record it as an explicit run event instead of hiding it inside queue hygiene
