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

**You are the supervisor, not a passive observer.** When you see a blocker, stop and fix it. Do not watch the same failure repeat.

## Red Lines

- **NEVER watch a repeating failure for more than 2 cycles.** If the same error appears twice, it will appear forever. Stop the conductor, diagnose, fix, restart.
- **NEVER file a blocker as a backlog item and keep running.** If it blocks the audit, it blocks the audit. Stop and fix it NOW, or declare the audit complete with the blocker as the finding.
- **NEVER run an overnight/extended monitoring loop on a factory that isn't doing productive work.** If no PR is opened within 15 minutes, something is wrong. Diagnose it.
- **The audit is DONE when you identify a critical blocker.** File it, fix it if you can, write the report. Don't continue "monitoring" a broken factory.

## Workflow

### 1. Preflight the control plane

- Read `CLAUDE.md`, `WORKFLOW.md`, and the target work item.
- Verify environment: `GITHUB_TOKEN`, `SPRITE_TOKEN`, Codex auth, fleet.toml.
- Pick a work item: scan `backlog.d/` for ready items sorted by priority. The operator may specify one directly.
- Check fleet health: `cd conductor && mix conductor fleet --fleet ../fleet.toml` to verify sprite reachability. If no sprite is healthy, record that as friction before fixing it.
- **Auth validation**: verify that auth tokens work for actual sprite operations, not just that env vars are set. If multiple sprites share auth credentials, verify they don't invalidate each other.

### 2. Launch the conductor

- Use `cd conductor && mix conductor start --fleet ../fleet.toml` to dispatch the fleet.
- Sprites pick their own work autonomously (Weaver → backlog.d/, Thorn → PR fixes, Fern → polish+merge).
- Record: start timestamp, fleet composition, target backlog item, sprite assignments.

### 3. Watch the run — actively, not passively

- Poll fleet health: `mix conductor fleet --fleet ../fleet.toml --json`
- Poll store events: `mix conductor show-events --limit 50`
- Check sprite logs for autonomous loop progress.
- Check GitHub for PRs opened, CI status, reviews, merge state.

**Active supervision means:**
- If a sprite exits within 5 minutes of launch, check WHY before letting it relaunch. Read the sprite logs. Is it auth? Context budget? No work? Each has a different fix.
- If the same `sprite_auth_failure` or `sprite_loop_exited` event appears for the same sprite twice in a row, STOP. The recovery loop is not solving the problem.
- If no sprite has opened a PR or pushed a branch within 15 minutes, check what they're actually doing. Read their logs. Are they reading files? Building? Stuck on auth?
- **Distinguish "factory infrastructure works" from "factory delivers work."** A factory that launches sprites, detects failures, and recovers them is NOT working if it never ships code.

### 4. Stop conditions — when to end the audit

**End immediately and write the report when:**
- A critical blocker is identified that prevents productive work (auth cascade, harness misconfiguration, broken bootstrap)
- A sprite successfully opens a PR (success — verify the PR quality and write the report)
- 30 minutes pass with no productive work and no new diagnostic information
- The same failure repeats 3+ times (the system is in a stable failure loop)

**Do NOT:**
- Set up a monitoring loop and walk away
- Watch the same auth failure cycle for hours
- File a blocker as a backlog item and continue monitoring
- Report "infrastructure is solid" when zero work was delivered

### 5. Fix blockers in-band when possible

If the blocker is fixable during the audit:
1. Stop the conductor
2. Fix the root cause (code change, config change, auth re-provision)
3. Run tests
4. Commit and push
5. Restart the conductor
6. Continue the audit with the fix in place

If the blocker requires investigation or design work beyond a quick fix:
1. Stop the conductor
2. File the backlog item with full evidence
3. Write the audit report
4. **The audit is complete.** Do not restart.

### 6. Verify the terminal state

- Confirm the backlog item, GitHub PR, health monitor state, and store events all agree.
- If the PR merged, verify whether it merged at the right time, not merely whether it merged.
- If a sprite failed or stalled, identify whether the conductor detected it (health transitions, events) and whether recovery was automatic.
- Check: did the health monitor detect loop exits? Did launch preflight clean stale processes? Did the tri-state (launching/healthy/unhealthy) track correctly?

### 7. Update the backlog

- Use `references/backlog-rules.md` to decide: new backlog item, update existing item, or no action.
- Create new `backlog.d/` items for real P0-P2 findings.
- Update existing items when the run provides stronger evidence, sharper scope, or revised priority.
- Do not create duplicate items for cosmetic nits unless they reveal a broader system pattern.

### 8. Leave a durable artifact

- Write a report from `templates/factory-audit-report.md`.
- Include: fleet composition, backlog item, PRs, timestamps, observed friction, and the exact backlog items created or updated.

## Key Principles

- **Supervisor, not observer.** You own the outcome. If the factory isn't delivering, that's YOUR problem to solve, not a finding to file.
- Delivery is not enough. A run that ships while being confusing, brittle, or overly manual is still a useful failure.
- **Non-delivery is a stop condition, not a monitoring condition.** If sprites aren't doing work, stop and figure out why.
- Separate evidence from interpretation. Capture timestamps, statuses, and artifacts before judging them.
- Prefer updating existing backlog items over creating duplicates when the problem is already known.
- The conductor is thin infrastructure — all judgment lives in sprites. Audit both layers: did the infrastructure keep sprites alive, and did the sprites make good decisions?

## Anti-Patterns (earned by pain)

- **The 20-hour watch.** Monitoring a factory that's cycling on auth failures for 20 hours, filing a backlog item, and calling the infrastructure "solid." The factory wasn't solid — it was broken. It just wasn't crashing.
- **Filing instead of fixing.** A blocker that you can fix in 30 minutes should be fixed during the audit, not filed for later.
- **"Infrastructure works" without delivery.** Health monitor transitions being correct is necessary but not sufficient. The audit succeeds when code ships.
- **Passive polling loops.** Setting up a cron to check every 15 minutes is monitoring, not supervision. Supervision means intervening when things go wrong.

## References

- `references/watchpoints.md` - phase-by-phase failure patterns and what to inspect
- `references/backlog-rules.md` - when to file, update, reprioritize, or ignore
- `templates/factory-audit-report.md` - durable report structure
