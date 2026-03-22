# Issue 778 Sprite Lifecycle Hardening Plan

> Scope issue: [#778](https://github.com/misty-step/bitterblossom/issues/778)

## Goal

Keep sprites serviceable without manual rebuilds by pruning stale checkpoints and probing cold sprites with a timeout that matches real boot behavior.

## Product Spec

### Problem

The conductor currently treats two recoverable sprite states as hard failures:

1. checkpoints accumulate until the sprite disk fills, which makes `sprite exec` fail before the command body runs
2. cold fixer/polisher sprites need longer than the default probe timeout to boot, so the proxy returns a transient 502 and the conductor marks them unreachable

Both cases block factory throughput even though the right fix belongs inside the existing sprite lifecycle surface.

### Intent Contract

- Intent: make sprite lifecycle repair truthful and automatic by pruning excess checkpoints and by distinguishing cold-boot latency from genuine reachability failure.
- Success Conditions: completed runs leave sprites below the checkpoint cap, the health monitor periodically re-applies that cap to healthy sprites, and cold sprites probe with a longer timeout while warm/running sprites keep the fast path.
- Hard Boundaries: keep the change inside existing `Conductor.Sprite`, `RunServer`, `HealthMonitor`, and configuration/test surfaces; do not add new processes or redesign fleet reconciliation.
- Non-Goals: build a full sprite dashboard, persist checkpoint inventories in SQLite, or change builder dispatch semantics outside the probe timeout.

## Technical Design

### Approach

1. Add `Conductor.Sprite.gc_checkpoints/2` as the deep-module entrypoint for listing checkpoints, keeping the newest N, and deleting the rest with best-effort logging.
2. Extend `Conductor.Sprite.probe/2` with a `state_fn`-driven timeout choice so cold sprites get a longer wake window and warm/running sprites keep the current fast probe.
3. Call checkpoint GC after run workspace cleanup and on a periodic cadence from `HealthMonitor` for all currently healthy sprites.
4. Cover the behavior with focused tests before touching the implementation so the timeout and cadence contracts are explicit.

### Files to Modify

- `conductor/lib/conductor/sprite.ex`
- `conductor/lib/conductor/config.ex`
- `conductor/lib/conductor/run_server.ex`
- `conductor/lib/conductor/fleet/health_monitor.ex`
- `conductor/test/conductor/sprite_test.exs`
- `conductor/test/conductor/sprite_health_test.exs`
- `conductor/test/conductor/config_test.exs`
- `conductor/test/conductor/fleet/health_monitor_test.exs`
- `conductor/test/conductor/run_server_test.exs`

### Implementation Sequence

1. Add failing tests for checkpoint pruning, probe timeout selection, health-monitor cadence, and run cleanup GC.
2. Implement checkpoint listing/deletion and state-aware probe logic in `Conductor.Sprite`.
3. Wire the post-run and periodic GC call sites.
4. Re-run focused Elixir tests, then expand verification if the slice stays clean.

### Risks & Mitigations

- Risk: the sprite API returns checkpoint/state payloads in slightly different shapes than the issue examples.
  Mitigation: normalize multiple plausible JSON shapes and fall back to conservative no-op behavior with a warning.
- Risk: periodic GC accidentally targets unhealthy or non-existent sprites.
  Mitigation: only run GC for sprites currently marked healthy in monitor state and keep failures best-effort.
- Risk: run cleanup tests start touching the real sprite CLI.
  Mitigation: keep the sprite module injectable in tests and assert the call contract directly.
