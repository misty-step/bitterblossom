# Retro

## 2026-03-12 [issue #481](https://github.com/misty-step/bitterblossom/issues/481)

- predicted: L
- actual: M
- scope: added a narrow telemetry sample ledger for builder/reviewer artifacts, rolled the data into the existing run inspection surfaces, and shipped one aggregate `show-metrics` read model instead of introducing a separate analytics service or dashboard backend
- blocker: walkthrough packaging needed a branch-scoped terminal artifact because the repo had no existing convention for internal PR evidence bundles
- pattern: when observability needs both per-run detail and trend reporting, append one deeper telemetry table to the existing ledger and derive operator read models from it instead of scattering metrics into ad hoc JSON blobs

## 2026-03-10 [issue #474](https://github.com/misty-step/bitterblossom/issues/474)

- predicted: L
- actual: M
- scope: replaced heuristic backlog sorting with a minimal readiness gate plus Claude-backed semantic routing, added a machine-readable route preview command, and tightened the issue contract/docs around autopilot-ready work
- blocker: live Claude CLI JSON mode returned an event stream envelope rather than a plain object, so the router needed a real-format parser fix before command verification passed
- pattern: when a control-plane decision is judgment-heavy, keep the deterministic boundary tiny and test the real structured-output envelope instead of rebuilding semantics with labels and timestamps

## 2026-03-12 [issue #538](https://github.com/misty-step/bitterblossom/issues/538)

- predicted: L
- actual: M
- scope: hardened the existing warm-mirror worktree lifecycle with serialized mutation, bounded preparation retries, and explicit operator-visible recovery state instead of adding a new workspace mechanism
- blocker: none
- pattern: when lifecycle truth is already in the run ledger, deepen the operator surface and failure contract before introducing more orchestration layers

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

## 2026-03-12 [issue #480](https://github.com/misty-step/bitterblossom/issues/480)

- predicted: L
- actual: M
- scope: added slot-aware worker capacity state to the conductor ledger, preserved the legacy single-slot routing seam for existing lanes, and shipped an operator-visible `show-workers` surface instead of inventing a separate dashboard
- blocker: true concurrent repo backfill remains a follow-up because the current conductor loop still executes one lane per process
- pattern: when a scheduler needs more depth, first persist the capacity model and operator truth in the existing control plane before attempting concurrent orchestration

## 2026-03-11 [issue #500](https://github.com/misty-step/bitterblossom/issues/500)

- predicted: L
- actual: M
- scope: extended duplicate suppression from PR-thread-only handling to all review findings and emitted explicit review-wave events so settlement becomes inspectable from the run store
- blocker: none
- pattern: if convergence is part of the product contract, record it directly in the ledger and event stream instead of inferring it from helper control flow

## 2026-03-11 [issue #482](https://github.com/misty-step/bitterblossom/issues/482)

- predicted: M
- actual: M
- scope: replaced the unsupported `nohup` coordinator story with one repo-owned supervisor script, a reboot launcher contract, bounded local log rotation, and explicit operator docs
- blocker: none
- pattern: when "always-on" behavior matters, ship a narrow supported runtime contract in code and docs instead of leaving operators to improvise shell folklore

## 2026-03-12 [issue #505](https://github.com/misty-step/bitterblossom/issues/505)

- predicted: L
- actual: M
- scope: added a narrow QA-intake lane that runs a configurable probe command, normalizes findings into deduped GitHub issues with stable evidence contracts, and gives same-tier routing preference to `source/qa`
- blocker: the existing worktree lock tests exposed shell-lock fragility, so the ship gate also required replacing that path with an inline Python `fcntl.flock` contract
- pattern: when a new intake source is still exploratory, keep the runner pluggable and codify only the stable handoff contract that the rest of the factory needs

## 2026-03-12 [issue #503](https://github.com/misty-step/bitterblossom/issues/503)

- predicted: L
- actual: M
- scope: added a durable repository registry to the conductor ledger, exposed `set-repo-state` and `show-repos`, and made run admission honor repo activation state plus desired concurrency before lease acquisition
- blocker: a stale lock-holder test helper had to be simplified before the full conductor suite would go green
- pattern: when scheduler policy is still evolving, persist the repo-level truth and expose it through existing operator surfaces before attempting full multi-repo orchestration
