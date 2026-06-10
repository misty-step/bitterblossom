# Self-reload after merge

Priority: critical
Status: done
Estimate: M

## Goal
When a PR merges to master that changes conductor code or sprite definitions, the factory must reload itself — pull latest, re-provision sprites with fresh code, and relaunch loops — without losing track of in-progress work or requiring operator intervention.

## Problem
Today, merged PRs sit on origin/master until the operator manually restarts the conductor. Sprites continue running with stale code and stale AGENTS.md until the next manual restart. This blocks the factory from improving itself through its own backlog.

## Design

### What triggers reload
- Conductor polls `origin/master` on each health check cycle (every 2 min)
- If `origin/master` has new commits since last check, trigger reload
- OR: GitHub webhook to conductor's Phoenix endpoint (lower latency, optional)

### Reload sequence
1. Record in-progress sprite states (which sprites are busy, what they're working on)
2. Wait for idle sprites to complete current work (don't interrupt active sessions)
3. Pull latest master on all sprite workspaces (`git pull --ff-only`)
4. Re-bootstrap spellbook on all sprites
5. For sprites that were idle: relaunch with fresh code immediately
6. For sprites that were busy: relaunch after current session ends (health monitor detects exit → relaunch picks up new code)

### What NOT to do
- Don't kill active sessions — let them finish. The next launch picks up new code.
- Don't restart the conductor process itself — just re-provision sprites. The conductor's Elixir code doesn't change as often as sprite definitions.
- Don't over-engineer: a simple `git pull` on sprite workspaces + re-bootstrap is sufficient.

## Sequence
- [ ] Add `origin/master` HEAD tracking to health monitor state
- [ ] On new commits detected: trigger re-provision for idle sprites
- [ ] Re-bootstrap spellbook after pull (picks up skill changes)
- [ ] Log reload events to Store
- [ ] Test: push a change to master, verify sprites pick it up within one health check cycle

## Oracle
- [ ] After a PR merges, idle sprites are re-provisioned with latest code within one health check cycle
- [ ] Active sprites finish their current work before reloading
- [ ] Spellbook is re-bootstrapped after reload
- [ ] No operator intervention needed for code reload
