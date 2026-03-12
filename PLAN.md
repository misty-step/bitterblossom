# PLAN: Issue #538 — Harden conductor worktree lifecycle

## Status

The core hardening is already implemented (PRs #549 and #555):
- `_mirror_locks` / `_mirror_lock()` — Python-level per-(sprite, mirror) serialization
- `prepare_run_workspace` with `WORKSPACE_PREP_RETRIES=2` retry policy
- `cleanup_builder_workspace` records `workspace_cleanup_failed` + `surviving_path`
- `show-runs` / `show-run` expose `worktree_path` in JSON output
- `docs/CONDUCTOR.md` documents all of the above under "Worktree Lifecycle and Serialization"

## Gap Identified

No test covers the cross-operation lock contention case: a concurrent
`prepare_run_workspace` and `cleanup_run_workspace` hitting the same
`_mirror_lock`. Existing tests cover prepare+prepare and cleanup bash-side flock,
but not the Python-level lock shared between prepare and cleanup.

## Steps

- [x] Audit existing implementation and tests
- [ ] Add `test_cleanup_run_workspace_serializes_with_prepare` to `test_conductor.py`
- [ ] Run `python3 -m pytest -q scripts/test_conductor.py -k "worktree or workspace or cleanup"`
- [ ] Push branch, open draft PR with `Closes #538`
- [ ] Write builder artifact

## Review

(To be filled after implementation)
