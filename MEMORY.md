# MEMORY

## Worktree lifecycle hardening (issue #538) — 2026-03-12

All acceptance criteria for #538 were implemented and merged across PRs #549, #555, #557:

- `_mirror_lock(sprite, mirror)` — per-(sprite, mirror) `threading.Lock` serializes concurrent in-process callers
- `prepare_run_workspace` retries up to `WORKSPACE_PREP_RETRIES=2` on `CmdError`, `OSError`, `TimeoutExpired`; lock released before retry sleep so other callers can proceed
- `cleanup_builder_workspace` records `workspace_cleanup_failed` + `surviving_path`; `worktree_path` column is not cleared on failure
- `show-runs` and `show-run` expose `worktree_path` in JSON output for lifecycle inspection
- `docs/CONDUCTOR.md` "Worktree Lifecycle and Serialization" section documents all of the above
- 30 targeted regression tests cover serialization, retry, cleanup-failure visibility, and command output

Run `python3 -m pytest -q scripts/test_conductor.py -k "worktree or workspace or cleanup"` to verify (30 passed, 0 failed).

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
