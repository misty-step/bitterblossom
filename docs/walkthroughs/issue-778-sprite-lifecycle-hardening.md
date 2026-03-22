# Issue 778 Walkthrough

## Goal

Keep sprites from degrading on routine lifecycle edges by pruning stale checkpoints and giving cold sprites enough time to boot before the conductor declares them unreachable.

## Before

- Sprite checkpoint growth was invisible to the conductor, so completed runs could keep stacking checkpoints until the overlay disk filled.
- `Conductor.Sprite.probe/2` always used a 15 second timeout, which was too short for cold fixer and polisher sprites.
- `RunServer` and `HealthMonitor` had no cleanup hook that pushed sprites back toward a healthy steady state after work completed.

## After

- `Conductor.Sprite.gc_checkpoints/2` lists checkpoints, keeps the newest configured set, and deletes stale ones through the sprite CLI.
- `RunServer` triggers checkpoint GC after workspace cleanup, and `HealthMonitor` re-applies the cap on a roughly 30 minute cadence for healthy sprites.
- `Conductor.Sprite.probe/2` now chooses a longer timeout for cold sprites while keeping the fast path for warm and running sprites.

## Verification

Run from [`conductor/`](/Users/phaedrus/.codex/worktrees/91b3/bitterblossom/conductor):

```bash
mix test test/conductor/config_test.exs test/conductor/sprite_test.exs test/conductor/sprite_health_test.exs test/conductor/fleet/health_monitor_test.exs test/conductor/run_server_test.exs
mix compile --warnings-as-errors
mix test
```

## Persistent Checks

- [`conductor/test/conductor/sprite_test.exs`](/Users/phaedrus/.codex/worktrees/91b3/bitterblossom/conductor/test/conductor/sprite_test.exs) proves checkpoint GC only deletes the oldest excess checkpoints.
- [`conductor/test/conductor/sprite_health_test.exs`](/Users/phaedrus/.codex/worktrees/91b3/bitterblossom/conductor/test/conductor/sprite_health_test.exs) proves cold sprites get the long probe timeout and warm sprites keep the fast path.
- [`conductor/test/conductor/fleet/health_monitor_test.exs`](/Users/phaedrus/.codex/worktrees/91b3/bitterblossom/conductor/test/conductor/fleet/health_monitor_test.exs) proves periodic GC fires on the expected cadence for healthy sprites.
- [`conductor/test/conductor/run_server_test.exs`](/Users/phaedrus/.codex/worktrees/91b3/bitterblossom/conductor/test/conductor/run_server_test.exs) proves successful runs trigger checkpoint GC for the worker sprite.

## Residual Risk

This lane validates the conductor logic against mocked sprite CLI responses, not against a live remote sprite fleet. The remaining risk is payload drift in the real sprite API or checkpoint command semantics, not the local orchestration path.
