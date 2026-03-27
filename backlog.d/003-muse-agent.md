# Muse agent: reflection + synthesis

Priority: medium
Status: ready
Estimate: M

## Goal
Fifth sprite (bb-muse) that runs `/reflect` after work cycles. Synthesizes learnings across agent runs, writes to backlog.d/ with improvement recommendations, detects recurring failure patterns.

## Non-Goals
- Real-time monitoring (that's HealthMonitor's job)
- Direct code changes (Muse observes and recommends, doesn't implement)

## Oracle
- [ ] sprites/muse/ has AGENTS.md with reflection loop
- [ ] fleet.toml declares bb-muse sprite
- [ ] Muse reads event logs and agent output, produces backlog items
- [ ] At least one backlog.d/ item created by Muse from a real run

## Notes
Corresponds to open issue #780.
