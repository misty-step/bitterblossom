# MEMORY

## Resolved Issues

### 2026-03-12 issue #538 — worktree lifecycle hardening

All acceptance criteria from #538 landed across PRs #549, #555, and #557:

- Mirror mutation serialized via `_mirror_lock` (Python) + `flock` (bash)
- `prepare_run_workspace` retries up to 2× on `CmdError`, `OSError`, `TimeoutExpired`
- Cleanup failures emit `workspace_cleanup_failed` event with `surviving_path`; `worktree_path` column not cleared so operators can recover
- `show-runs` / `show-run` surface `worktree_path` in JSON output
- `docs/CONDUCTOR.md` documents the lifecycle under "Worktree Lifecycle and Serialization"
- 30 regression tests cover all acceptance criteria paths

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
