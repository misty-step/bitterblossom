# Issue 787 Walkthrough

## Claim

Phase workers are now one shared deep module instead of two near-identical
singleton GenServers, so Thorn and Fern keep their current behavior while
multi-sprite-per-role becomes a fleet configuration change instead of another
copy-paste runtime.

## Before

- `Conductor.Fixer` and `Conductor.Polisher` each owned their own polling,
  task supervision, backoff, status, and completion loops.
- Application boot and `HealthMonitor` hardcoded singleton module names, so one
  new role or a second sprite for an existing role meant more boilerplate and
  more coupling.
- Status output could only talk about one fixer sprite and one polisher sprite.

## After

- `Conductor.PhaseWorker` owns the shared polling, dispatch, in-flight, and
  backoff behavior once.
- `Conductor.PhaseWorker.Roles.Fixer` and `.Polisher` hold only the role
  judgment: which PRs qualify, how to enrich prompt context, and how to
  dispatch.
- `Conductor.PhaseWorker.Supervisor` and a registry manage one worker per role
  with a sprite list, so health recovery updates the role pool instead of
  trying to start another singleton.
- CLI status and health recovery now report and manage the real role worker
  rather than hardcoded module names.

## Verification

Run from `conductor/`:

```bash
mix compile --warnings-as-errors
mix test test/conductor/phase_worker_test.exs test/conductor/fleet/health_monitor_test.exs test/conductor/application_test.exs
mix test
```

Observed:

- shared phase-worker tests prove fixer and polisher behavior still dispatches
  the right sprite/persona, backs off on failure, and fans out to two sprites
  in parallel
- health-monitor tests prove recovered sprites now join the existing role worker
- full suite passed: `511 tests, 0 failures`

## Persistent Checks

- `test/conductor/phase_worker_test.exs` protects the shared worker contract,
  including backoff and parallel multi-sprite dispatch.
- `test/conductor/fleet/health_monitor_test.exs` protects role-pool recovery.
- `test/conductor/application_test.exs` protects the operator role naming used
  in status surfaces.

## Residual Risk

This walkthrough proves the conductor-side refactor and runtime contracts, but
it does not exercise a live remote fleet with multiple Thorn/Fern sprites in
production. The remaining uncertainty is operational rollout rather than local
phase-worker logic.
