# Sprite heartbeat protocol

Priority: medium
Status: ready
Estimate: M

## Goal
Sprites emit structured heartbeat signals so the conductor can detect stuck-but-alive agents, not just dead processes. This is the semantic health layer that complements the process-level liveness in 004.

## Design
Each sprite writes a JSON heartbeat file (`~/.bitterblossom/loop-status.json`) containing:
- `status`: current phase (starting, observing, working, reflecting, idle)
- `ts`: Unix timestamp of last update
- `iteration`: loop iteration count
- `blocked_reason`: optional string if agent is stuck

The conductor probes this file alongside `loop_pid`. A stale heartbeat (>N minutes) with a live process indicates a stuck agent.

## Non-Goals
- Replace process-level liveness (that stays in the conductor via 004)
- Make sprites self-restart (conductor owns restart trigger)
- Fleet-wide semantic aggregation (that's a dashboard concern)

## Sequence
- [ ] Define heartbeat schema and write location
- [ ] Add heartbeat writes to sprite AGENTS.md loop definitions (start, each iteration, on error)
- [ ] Add `Sprite.heartbeat/2` to read and parse the heartbeat file
- [ ] Extend `HealthMonitor` probe to check heartbeat staleness alongside loop_pid
- [ ] Add `stale_heartbeat_threshold_ms` to fleet.toml config
- [ ] Test: sprite with stale heartbeat + live process is detected as degraded

## Oracle
- [ ] Each sprite role's AGENTS.md includes heartbeat writes at loop boundaries
- [ ] `Sprite.heartbeat/2` returns `{:ok, %{status, ts, iteration}}` or `{:error, reason}`
- [ ] A sprite with a heartbeat >threshold old is marked `:unhealthy` by HealthMonitor
- [ ] A freshly launched sprite that hasn't written a heartbeat yet is NOT marked unhealthy (grace period)
- [ ] `mix test` passes

## Notes
This is the C-flavored piece of the hybrid (Option D) architecture. Sprites own semantic health reporting; conductor owns the probe and restart decision. Depends on 004 being complete first.
