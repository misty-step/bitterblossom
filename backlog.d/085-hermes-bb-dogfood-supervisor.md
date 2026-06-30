# Add a durable Hermes-driven BB dogfood supervisor with explicit authority levels

Priority: P1 · Status: ready · Estimate: M

## Goal

Stop relying on an active chat turn or a single background watcher for BB dogfooding. Define and install a profile-scoped Hermes cron supervisor that can inspect BB state, report progress, and eventually dispatch the next safe BB action under explicit budget and authority caps.

## Problem / Dogfood Evidence

The 2026-06-30 overnight dogfood attempt did not loop. Hermes had no scheduled jobs in the active `urza` profile, while BB had one long-running builder run (`825ba972a832`) and a killed/superseded watcher process. The operator expected “rinse and repeat”; the actual system only had “one run plus one non-durable observer.”

This is partly Hermes configuration/ops and partly BB product design:

- Hermes cron was available but unused (`hermes -p urza cron list --all` returned no jobs).
- The chat session had no durable standing-goal runner after the turn ended.
- BB exposed run state, but not enough “last meaningful progress” to make the next automated decision confidently.
- Paid dispatch policy was implicit, so the agent avoided creating an autonomous spender but failed to say so or install a read-only monitor.

## Authority Levels

- **Level 0: read-only monitor** — inspect BB ledger/status, classify active/stale/completed runs, deliver a report. No dispatch, no git writes, no recovery, no paid runs.
- **Level 1: recovery/report helper** — may run `bb recover --json`, inspect artifacts, and write a local report/backlog note. No new agent dispatch.
- **Level 2: bounded continuation** — may dispatch at most one paid BB action per tick from a groomed allowlist, with daily cost/run caps and duplicate suppression.
- **Level 3: full dogfood loop** — may build, verify, storm, groom blockers, and select next backlog item, but still cannot merge or expand autonomy without scorecard approval.

## Oracle

- [ ] A Hermes cron job exists in the active `urza` profile for Level 0 read-only monitoring, delivered back to the operator channel.
- [ ] The cron prompt is self-contained: repo path, BB config path, allowed commands, stop conditions, and reporting schema.
- [ ] Level 0 report names active runs, terminal runs since last tick, stale candidates, dirty worktree state, and safe next action.
- [ ] Level 1/2/3 authority upgrades are separate backlog issues or explicit operator approvals, not silent prompt edits.
- [ ] The supervisor has hard caps: max new paid dispatches per tick/day, max storm fanout, max runtime, and stop-on-auth/dirtiness/stale-unknown conditions.
- [ ] The prompt records that Hermes cron sessions start fresh and cannot rely on current-chat context.
- [ ] `./scripts/verify.sh` passes after any repo-owned helper/scripts/docs are added.

## Verification System

- Claim: BB dogfood can continue overnight because Hermes has a durable supervisor with explicit authority and budget, not because a chat turn happened to leave a process running.
- Falsifier: cron list is empty; the job needs current chat context; it dispatches paid work at Level 0; it loops recursively scheduling more cron jobs; or it cannot classify a stuck running BB run.
- Driver: install Level 0 cron job, run it once manually, and inspect delivered report. Then simulate one terminal run and one stale running run in a fixture/local plane.
- Grader: report is accurate, bounded, and non-mutating at Level 0; higher-level prompts are not enabled without scorecard/operator approval.
- Evidence packet: `hermes -p urza cron list --all`, one `cron run` output, BB run/status JSON snippets, and this backlog ticket.
- Cadence: Level 0 every 30–60 minutes during active dogfood; pause when not dogfooding.

## Promotion Metrics

Level 0 → Level 1 only when:

- at least 5 monitor reports correctly classify run state without false action recommendations;
- no report requires shell spelunking beyond documented BB commands except known 079 artifact gap;
- stale-run threshold and safe-next-action language are operator-approved.

Level 1 → Level 2 only when:

- BB exposes enough artifact/progress state to inspect completed/stale runs without private path layout reliance, or the gap is explicitly accepted;
- duplicate active-run prevention exists or the supervisor enforces a deterministic idempotency key and active-run check;
- daily cost/run caps are configured and visible in the report.

Level 2 → Level 3 only when:

- at least three completed build→verify/storm cycles run under Level 2 without manual cleanup;
- blockers are shaped into backlog tickets with evidence;
- operator approves the next autonomy level.

## Notes

This ticket exists because the missing overnight loop was not just a missed cron command. It exposed a product boundary: durable orchestration, authority level, and budget policy must be first-class whenever we ask agents to keep building BB with BB.
