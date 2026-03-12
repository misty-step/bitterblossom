# PLAN: Issue #538 — Harden conductor worktree lifecycle under concurrency and transient failures

## Status: COMPLETE (implementation already in master via #549)

## Problem

The per-run worktree isolation from #469 fixed the shared-checkout contract but left
the lifecycle vulnerable under concurrent or degraded sprite conditions: mirror mutation
could race, transient failures left ambiguous state, and cleanup failures were silent.

## Acceptance Criteria Mapping

All four acceptance criteria are satisfied in the current codebase:

| Criterion | Location | Coverage |
|---|---|---|
| [test] Concurrent mirror mutation serialized | `prepare_run_workspace` + `_mirror_lock` | `test_prepare_run_workspace_serializes_overlapping_calls` |
| [test] Transient failure retries cleanly | `WORKSPACE_PREP_RETRIES` + retry loop | `test_prepare_run_workspace_retries_on_transient_failure`, `_exhausts_retries_with_explicit_message`, `_retries_on_timeout` |
| [behavioral] Cleanup failure is visible | `cleanup_builder_workspace` preserves `worktree_path` + emits `workspace_cleanup_failed` event | `test_cleanup_builder_workspace_records_workspace_cleanup_failed_on_error`, `_preserves_worktree_path_on_failure` |
| [command] `show-runs`/`show-run` include `worktree_path` | `serialize_run_surface`, `show_runs`, `show_run` | `test_show_runs_includes_worktree_path`, `test_show_run_includes_worktree_path` |

## Implementation Summary

- **In-process lock**: `_mirror_lock(sprite, mirror)` — one `threading.Lock` per (sprite, mirror) pair
- **Filesystem flock**: `flock --exclusive` on `.conductor_lock` inside the mirror dir — serializes concurrent processes
- **Retry policy**: `WORKSPACE_PREP_RETRIES = 2`, exponential backoff (`attempt + 1 * WORKSPACE_PREP_RETRY_DELAY_SECONDS`), retries on `CmdError`, `OSError`, `subprocess.TimeoutExpired`; exhausted retries raise with explicit message
- **Cleanup truth**: `cleanup_builder_workspace` records `workspace_cleanup_failed` event (with `surviving_path`) on failure; intentionally does not clear `worktree_path` column so operators can recover without reading the sprite filesystem
- **Inspector surface**: `show_runs` and `show_run` include `worktree_path` in JSON output; `show-events` reveals `workspace_cleanup_failed` event detail

## Docs

`docs/CONDUCTOR.md` "Worktree Lifecycle and Serialization" section documents the two-level lock strategy, retry policy, cleanup failure visibility, and manual cleanup procedure.

## Verification

```bash
python3 -m pytest -q scripts/test_conductor.py -k "worktree or workspace or cleanup"
# 28 passed
python3 -m pytest -q scripts/test_conductor.py
# 197 passed
python3 scripts/conductor.py show-runs --limit 20
python3 scripts/conductor.py show-run --run-id <run-id>
```
