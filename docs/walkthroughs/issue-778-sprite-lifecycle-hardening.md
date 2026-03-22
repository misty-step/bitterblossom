# Issue 778 Walkthrough

## Goal

Keep sprites from degrading on routine lifecycle edges by pruning stale checkpoints and auto-recreating sprites that match the stuck lifecycle signature.

## Before

- Sprite checkpoint growth was invisible to the conductor, so completed runs could keep stacking checkpoints until the overlay disk filled.
- Stuck sprites could sit in a `last_running_at: null` state until an operator destroyed and recreated them by hand.
- `RunServer` and `HealthMonitor` had no lifecycle hook that pushed sprites back toward a healthy steady state after work completed or after failed probes.

## After

- `Conductor.Sprite.gc_checkpoints/2` lists checkpoints, keeps the newest configured set, and deletes stale ones through the sprite CLI.
- `RunServer` triggers checkpoint GC after workspace cleanup, and `HealthMonitor` re-applies the cap on a roughly 30-minute cadence for healthy sprites.
- `Conductor.Sprite.check_stuck/2` inspects sprite API metadata and recreates sprites that are old enough and still report `last_running_at: null`.
- `HealthMonitor` checks failed probes for the stuck signature before marking the sprite degraded, then re-runs reconciliation after recreation.

## Verification

Run from `conductor/`:

```bash
mix test test/conductor/config_test.exs test/conductor/sprite_test.exs test/conductor/sprite_health_test.exs test/conductor/fleet/health_monitor_test.exs test/conductor/run_server_test.exs
mix compile --warnings-as-errors
mix test
```

## Persistent Checks

- `conductor/test/conductor/sprite_test.exs` proves checkpoint GC only deletes the oldest excess checkpoints.
- `conductor/test/conductor/sprite_health_test.exs` proves conservative probe timeouts still match sprite state.
- `conductor/test/conductor/fleet/health_monitor_test.exs` proves periodic GC cadence and stuck-sprite auto-recovery behavior.
- `conductor/test/conductor/run_server_test.exs` proves successful runs trigger checkpoint GC for the worker sprite.

## Residual Risk

This lane validates the conductor logic against mocked sprite CLI responses, not against a live remote sprite fleet. The remaining risk is payload drift in the real sprite API or checkpoint command semantics, not the local orchestration path.
