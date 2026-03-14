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

## Runtime Contract Codification (#606)

### 2026-03-14 — PR #639

**Canonical source pattern**: When a literal value (e.g. a model ID) must remain
consistent across Go code, shell scripts, and docs, the right pattern is:
1. Designate one file as the canonical source (`base/settings.json` in this case).
2. Extract a named constant in Go (`cmd/bb/runtime_contract.go`) that matches it.
3. Write a pytest test that reads the canonical source, then greps all other surfaces
   and asserts they match. This gives a clear failure message on drift.
4. Add the test directory to `pytest.ini` testpaths so it runs automatically.

**Real drift found**: `scripts/lib.sh` `openrouter-claude` fallback was
`"anthropic/claude-opus-4"` (different from the canonical `"anthropic/claude-sonnet-4-6"`
in `base/settings.json`). Always grep all surfaces — doc-only reviews miss code paths.

**pytest.ini testpaths**: Adding a new test directory requires updating `testpaths`
in `pytest.ini`. Without it, `pytest` (no args) won't discover the new tests.

## Phoenix LiveView Dashboard (#615)

### 2026-03-14 — PR #643, fix PR #645

**Re-run pattern**: Conductor may re-dispatch an issue if the original PR did not close
the issue (GitHub only auto-closes with "Closes #N" in the PR body, not just `(#N)` in
the title). When re-dispatched, assess what's in master before implementing — the work
may already be done. Look for genuine bugs to fix rather than adding busywork.

**cmd_dashboard escript startup bug (fixed in #645)**: `CLI.main/1` calls
`Application.ensure_all_started(:conductor)` before dispatching to sub-commands. The
supervisor starts without the endpoint (`:start_dashboard` is false at that point).
`cmd_dashboard` then sets `:start_dashboard` to true and calls `ensure_all_started`
again — but this is a no-op since the app is already running. The endpoint never starts.
Fix: use `Supervisor.start_child(Conductor.Supervisor, Conductor.Web.Endpoint)` after
configuring the endpoint env. Also: `Application.put_env` in `cmd_dashboard` REPLACES
the full key, so `adapter: Bandit.PhoenixAdapter` must be explicitly included or it
will be silently dropped from the running config.

**Module attribute ordering in Phoenix.Endpoint**: `@session_options` (or any
module attribute used inside `use Phoenix.Endpoint`) must be defined BEFORE the
`use` macro call, not after. The macro expansion happens at compile time and sees
nil if the attr is defined later.

**Bandit adapter must be explicit**: Phoenix 1.7+ now ships without Cowboy as a
default in newer versions (1.8+). When using Bandit, add
`adapter: Bandit.PhoenixAdapter` to both the app config and test config, or the
endpoint will try to use Plug.Cowboy and fail to start if cowboy isn't in deps.

**Phoenix.PubSub.broadcast takes atom name, not pid**: `Process.whereis/1`
returns a PID. `Phoenix.PubSub.broadcast/3` requires the registered atom name
(e.g. `Conductor.PubSub`), not the pid. Pattern: check `Process.whereis(Name)`
to guard the broadcast, then call `Phoenix.PubSub.broadcast(Name, ...)`.

**LiveView tests need lazy_html**: `Phoenix.LiveViewTest` since LiveView ~1.1
requires `{:lazy_html, ">= 0.1.0", only: :test}` in mix.exs. Without it,
`live/2` raises at runtime (not compile time).

**LiveView tests need Phoenix.ConnTest import**: `import Phoenix.LiveViewTest`
imports `live/2` but `live/2` internally calls `get/2` from `Phoenix.ConnTest`.
Must `import Phoenix.ConnTest` in the test module too.

**App env pollution across test modules**: Tests that call `Application.put_env`
must restore originals in `on_exit`. Non-async tests run sequentially but if a
prior test sets an env key and crashes before cleanup, the next module sees it.
Always save the original value before overwriting and restore in `on_exit`.

**Zero-build-complexity asset serving**: Serve phoenix.min.js and
phoenix_live_view.min.js directly from the deps' priv/static dirs using two
`Plug.Static` declarations (`from: {:phoenix, "priv/static"}` and
`from: {:phoenix_live_view, "priv/static"}`). No esbuild, no Node required.
