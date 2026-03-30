# Sprite session continuity

Priority: medium
Status: ready
Estimate: M

## Goal
When a sprite's session times out, the conductor automatically re-launches it. Sprites are stateless across sessions — they re-observe repo state on each start. The conductor's job is just to keep them alive.

## Non-Goals
- State persistence across sessions (sprites re-observe, don't resume)
- Infinite sessions (timeout is a safety net against runaway agents)
- Conductor-side retry logic or backoff (simple re-launch on exit)

## Sequence
- [ ] HealthMonitor detects sprite session ended (health probe returns stopped/exited)
- [ ] HealthMonitor triggers re-launch via Launcher (provision check → bootstrap → start_loop)
- [ ] Add `auto_restart: true | false` per sprite in fleet.toml (default true)
- [ ] Add circuit breaker: if a sprite crashes 3 times in 10 minutes, stop re-launching and emit a notification event
- [ ] Test: kill a sprite session, verify it restarts within one health check interval

## Oracle
- [ ] A sprite that times out is automatically re-launched within `fleet_health_check_interval_ms`
- [ ] `auto_restart: false` in fleet.toml prevents re-launch
- [ ] A sprite that crashes 3x in 10 minutes is NOT re-launched (circuit breaker)
- [ ] Circuit breaker emits a `sprite_circuit_open` event to PubSub
- [ ] `mix test` passes

## Notes
Reshaped from original "loop persistence" item. Under the thin-wrapper vision, this is simpler: the conductor doesn't manage loop state, it just notices a sprite stopped and re-starts it. HealthMonitor already has the detection; this adds the re-launch trigger.

Depends on 012 (kill orchestration layer) — the current auto-restart loop in Application is part of the orchestration that gets deleted. This item replaces it with a simpler HealthMonitor-driven approach.
