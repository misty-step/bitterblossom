# MEMORY

## Worktree Lifecycle Hardening (#538)

### 2026-03-13 — patterns locked in after PRs #545, #573, #574

**Serialization model**: one `flock` per repo mirror, shared by both
`prepare_run_workspace` and `cleanup_run_workspace`. Bash-level `exec 9>"$lockfile" && flock -w N 9`
with separate wait budgets (`WORKSPACE_PREPARE_LOCK_WAIT_SECONDS = 240`,
`WORKSPACE_CLEANUP_LOCK_WAIT_SECONDS = 120`). Tests verify that each
operation actually blocks when the other holds the lock.

**Retry clause**: `prepare_run_workspace_with_retry` catches `(CmdError,
subprocess.TimeoutExpired, OSError)`. OSError was added in PR #574 after
breaking-pipe transport errors fell outside the original `CmdError` catch.
Three attempts, 2-second delay, `workspace_preparation_retry` event per attempt,
then `workspace_preparation_failed` on exhaustion. Lease checked between attempts.

**Cleanup truth**: `cleanup_builder_workspace` catches all exceptions. On
failure it records a `cleanup_warning` event with `kind=builder_workspace_cleanup`
and leaves `worktree_path` **intact** in the run row so operators can recover
the surviving path from `show-run` without touching the sprite filesystem.
On success it clears `worktree_path` and writes `builder_workspace_cleaned`.

**Observer surface**: `show-runs` and `show-run` expose `worktree_path` plus
four recovery fields: `worktree_recovery_status`, `worktree_recovery_error`,
`worktree_recovery_event_type`, `worktree_recovery_event_at`. The status field
takes one of three values: `cleaned`, `cleanup_failed`, `prepare_failed`.

**Testing approach**: lock-contention tests use a real local git repo, a
`flock` shim that routes to the system binary, and `_hold_lock` (a background
subprocess) to hold the file descriptor. All lock-wait assertions check elapsed
≥ 1.0 s. This is the only reliable way to verify flock behavior without mocking
the entire bash execution path.

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

## Elixir CI Pipeline (#625)

### 2026-03-14 — PR #629

**OTP/Elixir compatibility**: Elixir 1.16 only supports OTP 24-26. OTP 27
requires Elixir 1.17+. When adding CI, always verify the version matrix at
https://hexdocs.pm/elixir/compatibility-and-deprecations.html. Use OTP 27 +
Elixir 1.18 for current projects with `~> 1.16` in mix.exs.

**Format before gating**: If existing code is not formatted and you're adding
`mix format --check-formatted` to CI, you MUST run `mix format` first to bring
the codebase into compliance. This is a prerequisite, not a scope violation —
the gate will never be green otherwise.

**Cache path with defaults working-directory**: When using `defaults.run.working-directory`,
`actions/cache@v4` paths must still be relative to the repo root (the cache step
doesn't inherit the working-directory default). Always use `conductor/deps` and
`conductor/_build` as cache paths when the job has `working-directory: conductor`.
