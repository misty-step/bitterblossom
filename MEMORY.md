# MEMORY

## Filed Issues

### 2026-02-18 e2e-test shakedown

- #410 `fix: logs --json must emit parseable JSON only` (NEW)
- #411 `fix: fleet status should surface busy sprites before dispatch` (NEW)
- #405 commented (expired token error still misleading)
- #407 commented (need true dispatch dry-run)
- #285 commented (missing TASK_COMPLETE regression signal)
- #293 commented (wait/poll hang regression signal)
- #320 commented (json-output pollution class)
- #388 commented (streaming regression signal)

## Issue #538 — Worktree lifecycle hardening (2026-03-12)

Resolved via PRs #549, #555, #557 (merged to master). Implementation:

- Python-level `threading.Lock` per `(sprite, mirror)` serializes in-process concurrent ops
- Filesystem `flock --exclusive` on `.conductor_lock` serializes cross-process access
- `prepare_run_workspace` retries up to `WORKSPACE_PREP_RETRIES=2` on transient `CmdError` or `OSError`
- `cleanup_builder_workspace` records `workspace_cleanup_failed` event with `surviving_path`; `worktree_path` is *not* cleared on failure so operators can recover
- `show-runs` / `show-run` expose `worktree_path` in JSON output
- Tests cover: flock presence, serialized overlapping calls, cross-op prepare+cleanup lock, retry on transient and OS errors, cleanup failure visibility, and worktree inspection commands

### Pattern: stale conductor runs on closed issues

The conductor can acquire a lease and dispatch a builder even after the underlying
GitHub issue has been closed (e.g., if the issue was closed between lease acquisition
and builder dispatch, or via a direct `gh issue close`). When a builder finds all
acceptance criteria already satisfied on master, the correct move is to verify tests
pass, write the artifact with `status=ready_for_review`, and let the governance lane
close the loop. Do not skip the artifact write — the conductor needs it to release
the lease cleanly.
