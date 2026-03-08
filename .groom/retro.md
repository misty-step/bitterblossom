# Retro

## 2026-03-07 [issue #499](https://github.com/misty-step/bitterblossom/issues/499)

- predicted: M
- actual: M
- scope: paired the ADR with the storage foundation so review governance has both policy and substrate in one reviewable slice
- blocker: none
- pattern: make governance append-only first, then move merge policy onto that ledger in follow-up issues

## 2026-03-08 [issue #98](https://github.com/misty-step/bitterblossom/issues/98)

- predicted: M
- actual: M
- scope: kept the issue run-centric and limited the slice to truthful operator surfaces plus live heartbeats during long builder/CI/external-review waits
- blocker: detached HEAD worktree needed direct git access for branch and commit writes
- pattern: derive operator answers from durable run and event state, not whichever event landed last
