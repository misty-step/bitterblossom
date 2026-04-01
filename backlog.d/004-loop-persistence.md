# Conductor loop liveness and launch safety

Priority: critical
Status: in-progress
Estimate: M

## Goal
Close the three gaps exposed by the orchestration-layer refactor (012):
1. Sprites marked healthy before loop starts — failed launches never retried
2. Health ignores loop liveness — crashed/timed-out sessions never restarted
3. No launch preflight — stale processes and pid files block relaunch

## Design: Hybrid responsibility split

**Conductor owns** (process-level, can't be in-band):
- Launch state tracking (`:launching` → `:healthy` → `:unhealthy`)
- Process-level liveness detection (`loop_pid` alive?)
- Launch preflight (kill stale processes, clear pid/lock artifacts)
- Restart trigger with backoff

**Sprites own** (semantic, requires agent judgment):
- Progress heartbeats (follow-on: 016-sprite-heartbeat-protocol)
- Role-specific workspace cleanup
- Recovery classification (retry vs escalate)

## Sequence
- [ ] Add `loop_alive` field to `Sprite.status/2` (derived from `loop_pid != nil`)
- [ ] Upgrade `HealthMonitor` from binary `:healthy/:unhealthy` to tri-state `:launching/:healthy/:unhealthy`
- [ ] Seed launched sprites as `:launching` in `Application.launch_with_config/2`
- [ ] Add `stop_loop/2` preflight in `Launcher.launch/3` before `start_loop`
- [ ] Add launch timeout: if sprite stays `:launching` for N probe cycles, mark `:unhealthy`
- [ ] Distinguish clean exit (no work) from crash exit — back off relaunch when sprites repeatedly exit cleanly within one health cycle
- [ ] Tests for: failed launch, loop death, stale process cleanup, launch timeout, clean-exit backoff

## Oracle
- [ ] A sprite whose launch fails is NOT marked `:healthy` — it stays `:launching` then degrades to `:unhealthy`
- [ ] A sprite whose loop exits is detected within one health check interval and relaunched
- [ ] Stale agent processes from a previous run are killed before a new loop starts
- [ ] A sprite stuck in `:launching` for >=3 probe cycles transitions to `:unhealthy`
- [ ] `mix test` passes
- [ ] `mix compile --warnings-as-errors` passes

## Evidence (2026-04-01 factory audit)
bb-fixer and bb-polisher exit within ~4 min of launch when no PRs exist. HealthMonitor detects exit, relaunches immediately, creating an infinite restart cycle. Each cycle burns ~4 min of context-reading and API tokens. The health monitor correctly detects the exit but doesn't distinguish "no work available" from "crashed loop" — both get the same immediate relaunch treatment.

## Notes
Replaces the old "sprite session continuity" item. The analysis (adversarial review + thinktank + codex) concluded:
- Process liveness must stay in the conductor — dead agents can't self-heal
- Semantic health should move to sprites (follow-on item)
- A janitor sprite would just recreate the orchestration layer in a different costume
- The fixes are small: existing signals (`loop_pid`, `stop_loop/2`) just need wiring
