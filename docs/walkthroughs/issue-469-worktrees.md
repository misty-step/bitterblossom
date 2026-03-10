# Walkthrough: Issue 469

## Title

Isolate conductor builder and reviewer runs with per-run Git worktrees while preserving the warm repo mirror.

## Why Now

Before this branch, the conductor dispatched builders and reviewers into the shared checkout at `/home/sprite/workspace/<repo>`. That violated ADR-003's isolation rule, let untracked files leak across runs, and made cleanup dependent on a dirty shared tree staying healthy.

## Before

```mermaid
flowchart TD
    A["warm shared checkout"] --> B["builder dispatch"]
    A --> C["reviewer dispatch"]
    B --> D["artifacts written inside shared tree"]
    C --> D
    D --> E["next run reuses same checkout"]
```

- Conductor artifact paths were run-scoped, but the execution surface was not.
- `bb dispatch` always synced and ran inside `/home/sprite/workspace/<repo>`.
- `show-runs` could not tell an operator which worktree a run used because no worktree path was persisted.

## What Changed

```mermaid
flowchart TD
    A["warm mirror /home/sprite/workspace/<repo>"] --> B["builder worktree .bb/conductor/<run>/builder-worktree"]
    A --> C["review worktree .bb/conductor/<run>/review-<reviewer>-worktree"]
    B --> D["bb dispatch --workspace <builder worktree>"]
    C --> E["bb dispatch --workspace <review worktree>"]
    D --> F["run store persists builder worktree_path"]
    E --> G["review worktrees cleaned after round"]
    F --> H["show-runs exposes worktree_path"]
```

- `scripts/conductor.py` now prepares and removes run-scoped worktrees off the warm mirror.
- `cmd/bb/dispatch.go` accepts `--workspace` so the conductor can dispatch into an already-prepared worktree without re-syncing the shared checkout.
- The run ledger now stores `worktree_path`, and the operator docs explain how to inspect it with `show-runs`.

## After

```mermaid
stateDiagram-v2
    [*] --> MirrorReady
    MirrorReady --> BuilderWorktreePrepared
    BuilderWorktreePrepared --> Building
    Building --> Reviewing
    Reviewing --> ReviewWorktreesCleaned
    ReviewWorktreesCleaned --> MergeOrBlock
    MergeOrBlock --> BuilderWorktreeCleaned
    BuilderWorktreeCleaned --> [*]
```

Observable improvements:

- consecutive runs no longer share the same mutable execution directory
- reviewer workspaces are isolated from the builder and from each other
- the coordinator has durable metadata showing which builder worktree belonged to a run

## Verification

Primary protecting checks:

- `python3 -m pytest -q scripts/test_conductor.py`
- `go test ./...`

Evidence covered by those checks:

- conductor dispatch plumbing accepts and propagates workspace overrides
- builder run state records `worktree_path`
- `show-runs` surfaces `worktree_path`
- worktree helper paths and cleanup behavior are exercised in regression tests

## Residual Risk

- The walkthrough proves the control-plane contract and local test coverage, not a live sprite integration run against a real remote worker.
- `worktree_path` is currently persisted for the builder lane; reviewer paths are intentionally ephemeral and only visible through events/logs.

## Merge Case

This branch closes the largest remaining isolation gap in the conductor MVP without adding a second sync mechanism or abandoning warm mirrors. It makes run state more truthful, cleanup more deterministic, and the worker filesystem contract inspectable by operators.
