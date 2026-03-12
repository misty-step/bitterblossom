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

## 2026-03-10 [issue #469](https://github.com/misty-step/bitterblossom/issues/469)

- predicted: L
- actual: M
- scope: moved builder and reviewer execution off the shared checkout by threading run-scoped worktrees through conductor dispatch, run state, and operator docs
- blocker: walkthrough and PR packaging took longer than the implementation because the repo had no existing walkthrough artifact convention
- pattern: keep transport generic with one `--workspace` override, then let the conductor own run isolation and cleanup policy

## 2026-03-10 [issue #479](https://github.com/misty-step/bitterblossom/issues/479)

- predicted: L
- actual: M
- scope: split the conductor into an explicit builder handoff plus governor adoption path, added a minimum-age merge freshness gate, and forced one final polish pass before merge
- blocker: none
- pattern: when one loop owns both production and governance, carve out the adoption boundary in persisted state first so delayed-merge policy can evolve without forking the ledger

## 2026-03-11 [issue #500](https://github.com/misty-step/bitterblossom/issues/500)

- predicted: L
- actual: M
- scope: extended duplicate suppression from PR-thread-only handling to all review findings and emitted explicit review-wave events so settlement becomes inspectable from the run store
- blocker: none
- pattern: if convergence is part of the product contract, record it directly in the ledger and event stream instead of inferring it from helper control flow
