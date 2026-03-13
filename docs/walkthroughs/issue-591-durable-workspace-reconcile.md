# Issue 591 Walkthrough: Durable Workspace Reconcile Cleanup

Issue [#591](https://github.com/misty-step/bitterblossom/issues/591) requires one work item to own one durable workspace until the run is either cleanly released or explicitly left for operator recovery.

This branch closes the gap where `reconcile-run` could mark a run merged or closed without attempting the normal builder workspace cleanup path. That left a terminal run attached to a stale durable workspace even though the active governor path would have tried to release it.

## Before

- `run-once` created a builder workspace and persisted `worktree_path`
- `govern-pr` reused that same workspace for repair and final polish
- `reconcile-run` only updated run status fields when a PR was already merged or closed
- a reconciled terminal run could therefore keep a stale `worktree_path` without any cleanup attempt

## After

- `reconcile-run` now loads the stored builder sprite and `worktree_path`
- merged and closed PR reconciliation runs through the existing builder workspace cleanup helper
- successful reconcile cleanup clears `worktree_path` and records `builder_workspace_cleaned`
- failed reconcile cleanup preserves `worktree_path` and records the standard `cleanup_warning` recovery context

## Verification

```bash
python3 -m pytest -q scripts/test_conductor.py -k 'reconcile_run or cleanup_builder_workspace or worktree_recovery'
python3 -m pytest -q scripts/test_conductor.py
ruff check scripts/conductor.py scripts/test_conductor.py
```

## Reviewer Notes

- Persistent verification: the focused reconcile/worktree pytest slice above
- Strongest evidence: the full `scripts/test_conductor.py` suite stays green with terminal reconcile cleanup enabled
- Residual risk: runs missing `builder_sprite` can only record a cleanup warning because the conductor lacks the sprite identity needed to remove the worktree remotely
