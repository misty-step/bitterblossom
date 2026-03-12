# PLAN: Issue #538 — Harden conductor worktree lifecycle

## Status

The core hardening is already in master (PR #545):
- Bash-level `flock` per-repo mirror lock serializing fetch, worktree add/remove, and prune
- `prepare_run_workspace_with_retry` with 3 attempts and explicit `workspace_preparation_failed` events
- `cleanup_builder_workspace` records `cleanup_warning` + preserves `worktree_path` for operator recovery
- `show-runs` / `show-run` expose `worktree_path` and `worktree_recovery_*` fields
- `docs/CONDUCTOR.md` documents worktree lifecycle semantics and operator recovery

## Gap Identified

No test explicitly verified the cross-operation lock contention case: that
`cleanup_run_workspace` must wait when the per-repo mirror lock is held by a
concurrent `prepare_run_workspace` (or any other mirror mutation). The existing
tests covered prepare+external-lock-holder and cleanup+lock-timeout, but not
the symmetric case that proves cleanup and prepare share and respect the same flock.

## Steps

- [x] Audit existing implementation and tests
- [x] Add `test_cleanup_run_workspace_waits_for_lock_release` to `scripts/test_conductor.py`
- [x] Run `python3 -m pytest -q scripts/test_conductor.py -k "worktree or workspace or cleanup"` — 36 passed
- [x] Run full `python3 -m pytest -q scripts/test_conductor.py` — 237 passed
- [x] Push branch, open draft PR with `Closes #538`
- [x] Write builder artifact

## Review

- New test `test_cleanup_run_workspace_waits_for_lock_release` reuses the `_init_local_worktree_mirror`,
  `_install_flock_shim`, `_local_sprite_bash`, and `_hold_lock` helpers already in place for the
  prepare+lock tests.
- Cleanup first creates a real worktree via prepare, then a lock holder simulates a concurrent
  prepare on the same sprite mirror. Cleanup must demonstrably block for ≥1 second before the
  lock holder releases and cleanup completes.
- All 237 tests pass.
