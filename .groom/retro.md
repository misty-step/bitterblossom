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
- scope: tightened the run-centric operator surface so show-runs exposes heartbeat age and blocking reason, while show-events bundles run metadata with recent context and preserves legacy jsonl output
- blocker: none
- pattern: operator summaries should derive from stable run truth, not only from whatever event slice happens to be visible
