# Factory Audit Report

## Summary

- Date: 2026-03-19
- Primary Run ID: run-702-1773928307
- Issue: #702 — [P1] Delete dead scripts/ — bury Gen 1 and Gen 2 shell layer
- PR: #735 — fix: retire dead shell entrypoint references
- Worker: bb-builder
- Reviewers: Cerberus (github-actions), CodeRabbit, Gemini Code Assist, Fern (bb-polisher)
- Terminal State: **MERGED** — issue closed, PR merged, commit 655ea75 on master

Secondary runs observed: run-703 (#703 → PR #736), run-707 (#707), run-708 (#708)

## Timeline

| Time (UTC) | Event | Notes |
|------------|-------|-------|
| 13:47:26 | Conductor boot attempt 1 | bb-builder unreachable (mid-restart) |
| 13:49:45 | Conductor boot attempt 2 | bb-builder provisioning failed (CWD mismatch, #708) |
| 13:50:30 | Manual `bb setup` from repo root | Workaround for #708 |
| 13:51:25 | Conductor boot attempt 3 | 3 healthy sprites, 3 stale runs expired |
| 13:51:25 | Self-update | Pulled PR #727 changes, recompiled |
| 13:51:47 | **run-702 lease acquired** | Issue #702 |
| 13:51:48 | **run-703 lease acquired** | Issue #703, 123ms after run-702 (race, #709) |
| 13:51:48 | Shaper triggered for #734 | Missing `## Problem` section |
| 13:51:59 | Shaper completed for #734 | Issue shaped successfully |
| ~13:58 | PR #735 opened | Builder pushed during codex session |
| 14:08:53 | **run-702 builder complete** | 17 min build time, PR detected |
| 14:08:53 | Workspace cleaned for run-702 | |
| 14:11:36 | Fern dispatched to PR #735 | Green CI detected |
| ~14:13 | Fern added LGTM to PR #735 | ~2 min polish |
| 14:14:09 | run-703 builder complete | 22 min build time, PR #736 detected |
| 14:14:18 | **PR #735 merged** | Orchestrator: lgtm + green CI |
| 14:14:21 | **run-702 terminated: merged** | |
| 14:14:22 | Self-update (PR #735 merge) | Recompiled with new code |
| 14:14:31 | Retro complete for run-702 | "clean Gen 1/2 removal - no architectural gaps" |
| 14:16:35 | Thorn dispatched to PR #737 | Non-conductor PR, red CI |
| 14:18:39 | Fern dispatched to PR #736 | Green CI after Thorn fix |
| 14:20:18 | Thorn completed PR #737 | |
| 14:31:59 | New runs dispatched (#707, #708) | Two concurrent again (#709) |
| 14:33:41 | Fern completed PR #736 | **No LGTM** — 8 unresolved threads |

## Findings

### Finding 1: Fleet sprite names out of sync with Fly machines

- Severity: P0 (→ filed as P1, since fleet.toml is operator config)
- New issue: **#741**
- Observed: fleet.toml had bb-weaver/bb-thorn/bb-fern/bb-muse; Fly machines are bb-builder/bb-fixer/bb-polisher
- Expected: Names match between config and infrastructure
- Why it matters: Factory completely broken — conductor can't reach any sprite
- Evidence: `sprite ls -o misty-step` vs fleet.toml contents

### Finding 2: Reconciler marks sprites permanently degraded if mid-restart

- Severity: P1
- New issue: **#742**
- Observed: bb-builder unreachable during 15s probe (0 min uptime = Fly restart), orchestrator never starts
- Expected: Recovery within 1-2 poll cycles
- Why it matters: Any transient sprite unreachability requires full conductor restart
- Evidence: Boot logs showing "no healthy weavers — orchestrator will not poll"

### Finding 3: bb setup CWD mismatch blocks fleet reconciliation

- Severity: P1
- Existing issue: **#708** (commented)
- Observed: `open base/settings.json: no such file or directory` when reconciler runs from conductor/
- Expected: `bb setup` works regardless of CWD
- Why it matters: Sprites that lose auth can't be auto-provisioned; requires manual intervention
- Evidence: Reconciler error log, manual workaround required

### Finding 4: Two concurrent runs dispatched to same sprite

- Severity: P1
- Existing issue: **#709** (commented)
- Observed: Runs 702 and 703 dispatched within 123ms; later, runs 707 and 708 same pattern
- Expected: One run per sprite at a time
- Why it matters: Two codex processes compete for CPU/memory; unpredictable behavior
- Evidence: `ps aux` showing PIDs 1333 and 1635 on bb-builder

### Finding 5: No builder progress visibility

- Severity: P2
- Existing issue: **#714** (commented)
- Observed: 17-minute silence between dispatch and builder_complete
- Expected: Periodic heartbeat events showing builder is alive and making progress
- Why it matters: Operator can't distinguish stuck builder from working builder without SSH
- Evidence: Conductor log gap from 13:51 to 14:08

### Finding 6: Log noise from busy deferrals

- Severity: P2
- Existing issue: **#712** (commented)
- Observed: 1,151 log lines in 28 minutes; 97% noise after filtering
- Expected: One summary line per poll cycle, not per-issue noise
- Why it matters: Signal buried in noise; operator cannot read logs
- Evidence: 12 eligible issues × ~28 poll cycles × 2 lines each

### Finding 7: Polisher cannot resolve external review threads

- Severity: P2
- New issue: **#743**
- Observed: Fern completed 15-min review of PR #736 without LGTM; 8 unresolved threads
- Expected: Either resolve threads or escalate to operator
- Why it matters: PRs with external review feedback sit indefinitely
- Evidence: PR #736 review thread analysis showing 8 unresolved from Cerberus/CodeRabbit

## Backlog Actions

- New issues filed: **#741** (fleet names), **#742** (builder recovery), **#743** (polisher threads)
- Existing issues commented: **#708**, **#709**, **#712**, **#714**
- Priority changes: none

## Reflection

### What Bitterblossom did well

- **Full pipeline worked end-to-end.** Issue #702 went from open to merged in 23 minutes with zero human intervention. Builder → Fern → merge, clean lifecycle.
- **Self-update is elegant.** The conductor pulled PR #727 changes and recompiled live. When PR #735 merged, it self-updated again. Hot code reload in production.
- **Thorn is effective.** Fixed CI on PR #736 (Elixir check failure) and PR #737 (non-conductor PR). Cross-PR awareness is a strength.
- **Shaper caught under-specified issues.** Issue #734 was missing `## Problem` and was shaped before dispatch. Gate works.
- **Retro ran automatically.** "Clean Gen 1/2 removal - no architectural gaps detected" — useful signal.
- **Stale run reconciliation.** Three orphan runs from yesterday expired automatically on boot.

### What felt brittle

- **Three boot attempts.** Transient sprite restart + CWD mismatch + name mismatch required 3 tries and manual intervention.
- **Single builder bottleneck.** All 12 eligible issues deferred because bb-builder was busy. With one sprite, the factory is serial.
- **97% log noise.** The conductor's own output is unreadable during normal operation.
- **No actor for review feedback.** External review threads create a dead-end in the pipeline.

### What should be simpler next time

- Fleet health recovery should be automatic, not manual.
- One sprite = one run. The busy check should be in-memory, not a remote pgrep race.
- Log at summary level, not per-issue.
- The polisher should either resolve threads or tell the operator what's blocking.
