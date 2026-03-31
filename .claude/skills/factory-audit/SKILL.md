---
name: factory-audit
description: |
  Supervise one Bitterblossom conductor run as a live end-to-end test, then turn observed friction into backlog updates. Use when running a Bitterblossom issue deliberately, watching builder/reviewer/CI/merge behavior step by step, validating governance, or reflecting on whether the factory is getting better or worse. Keywords: supervised run, factory audit, conductor shakedown, trace bullet, run-once, backlog feedback, issue triage, merge policy, worker health, observability.
allowed-tools:
  - Read
  - Grep
  - Glob
  - Bash
---

# Factory Audit

Run one Bitterblossom work item through the conductor as a supervised exercise. Treat the run as both delivery and diagnosis: prove the work ships, then prove the factory handled it elegantly.

## Workflow

### 1. Preflight the control plane

- Read `CLAUDE.md`, `WORKFLOW.md`, and the target work item.
- Verify environment: `GITHUB_TOKEN`, `SPRITE_TOKEN`, Codex auth, fleet.toml.
- Pick a work item: scan `backlog.d/` for ready items sorted by priority. The operator may specify one directly.
- Check fleet health: `cd conductor && mix conductor fleet --fleet ../fleet.toml` to verify sprite reachability. If no sprite is healthy, record that as friction before fixing it.

### 2. Launch the conductor

- Use `cd conductor && mix conductor start --fleet ../fleet.toml` to dispatch the fleet.
- Sprites pick their own work autonomously (Weaver → issues, Thorn → PR fixes, Fern → polish+merge).
- Record: start timestamp, fleet composition, target backlog item, sprite assignments.

### 3. Watch the run continuously

- Poll fleet health: `mix conductor fleet --fleet ../fleet.toml --json`
- Poll store events: `mix conductor events --limit 50`
- Check sprite logs for autonomous loop progress.
- Check GitHub for PRs opened, CI status, reviews, merge state.
- Treat silence, lag, stale health state, failed launches, loop exits, dirty workspaces, or misleading output as findings even when the run eventually succeeds.

### 4. Verify the terminal state

- Confirm the backlog item, GitHub PR, health monitor state, and store events all agree.
- If the PR merged, verify whether it merged at the right time, not merely whether it merged.
- If a sprite failed or stalled, identify whether the conductor detected it (health transitions, events) and whether recovery was automatic.
- Check: did the health monitor detect loop exits? Did launch preflight clean stale processes? Did the tri-state (launching/healthy/unhealthy) track correctly?

### 5. Update the backlog

- Use `references/backlog-rules.md` to decide: new backlog item, update existing item, or no action.
- Create new `backlog.d/` items for real P0-P2 findings.
- Update existing items when the run provides stronger evidence, sharper scope, or revised priority.
- Do not create duplicate items for cosmetic nits unless they reveal a broader system pattern.

### 6. Leave a durable artifact

- Write a report from `templates/factory-audit-report.md`.
- Include: fleet composition, backlog item, PRs, timestamps, observed friction, and the exact backlog items created or updated.

## Key Principles

- Separate evidence from interpretation. Capture timestamps, statuses, and artifacts before judging them.
- Delivery is not enough. A run that ships while being confusing, brittle, or overly manual is still a useful failure.
- Prefer updating existing backlog items over creating duplicates when the problem is already known.
- The conductor is thin infrastructure — all judgment lives in sprites. Audit both layers: did the infrastructure keep sprites alive, and did the sprites make good decisions?

## References

- `references/watchpoints.md` - phase-by-phase failure patterns and what to inspect
- `references/backlog-rules.md` - when to file, update, reprioritize, or ignore
- `templates/factory-audit-report.md` - durable report structure
