# Factory Audit Report

## Summary

- Date: 2026-04-01
- Backlog item: none targeted (empty-queue audit)
- Fleet: 3 sprites (bb-builder/Weaver, bb-fixer/Thorn, bb-polisher/Fern)
- Sprites dispatched: 3
- PRs opened: 0
- Terminal state: all sprites healthy but idle/restart-looping on empty queues

## Timeline

| Time (local) | Event | Notes |
|------|-------|-------|
| 10:39:00 | Conductor start (attempt 1) | Auth checks pass (false-green) |
| 10:39:00 | All 3 sprites unreachable | Fly auto_stop machines |
| 10:39:00–04 | 3x3 wake attempts fail | sprite CLI keyring auth expired |
| 10:39:04 | All 3 degraded | "no healthy workers" |
| 10:39:04 | HealthMonitor takes over | Retries same failing wake |
| 10:41–10:48 | HealthMonitor retries fail | Same auth error, 2-min intervals |
| 10:48 | Conductor stopped manually | Operator intervention required |
| 10:49 | `sprite login -o misty-step` | Operator re-authenticates CLI |
| 10:52:08 | Conductor start (attempt 2) | |
| 10:52:15 | bb-polisher healthy (7s) | |
| 10:52:31 | bb-builder healthy (23s) | |
| 10:52:46 | bb-fixer healthy (38s) | All 3 reconciled |
| 10:52:46 | 3 loops launching | |
| 10:52:51–53 | Spellbook bootstrapped | All 3 repo checkouts reprovisioned |
| 10:53:04 | bb-builder loop started | |
| 10:53:06 | bb-fixer loop started | |
| 10:53:07 | bb-polisher loop started | |
| 10:54:51 | bb-builder loop confirmed | First health check cycle |
| 10:54:56 | bb-fixer loop confirmed | |
| 10:55:00 | bb-polisher loop confirmed | |
| 10:57:11 | bb-fixer loop exited | No open PRs to fix |
| 10:59:34 | bb-fixer recovered + relaunched | Health monitor recovery |
| 10:59:41 | bb-polisher loop exited | No open PRs to polish |
| 10:59:44 | bb-fixer loop started (2nd) | |
| 11:01:52 | bb-fixer loop confirmed (2nd) | |
| 11:01:59 | bb-polisher recovered + relaunched | |
| 11:02:07 | bb-polisher loop started (2nd) | |
| 11:04:09 | Conductor stopped | Audit complete |

## Health Monitor Observations

- Initial health states: all 3 `:launching` after reconciliation
- Transitions observed:
  - `:launching` -> `:healthy` (all 3, within 7-38s of reconciliation)
  - `:healthy` -> `:unhealthy` via `sprite_loop_exited` (bb-fixer at 10:57, bb-polisher at 10:59)
  - `:unhealthy` -> `:launching` via `sprite_recovered` (bb-fixer at 10:59, bb-polisher at 11:01)
  - Second cycle: bb-fixer confirmed healthy again, bb-polisher relaunching
- Stuck states: bb-builder stayed "healthy/idle" throughout — its Codex process lingered even after completing work
- Store events correctly recorded all transitions (IDs 4671-4678)

## Findings

### Finding: check-env false-green on sprite CLI auth

- Severity: P0/critical
- Existing backlog item or new: **new** — `backlog.d/017-check-env-false-green.md`
- Observed: `check-env` passes when `FLY_API_TOKEN` is set, conductor starts, all sprite operations fail with "no token found for organization misty-step"
- Expected: `check-env` should fail if sprite CLI auth is expired, preventing a doomed conductor start
- Why it matters: false success signal — operator trusts "all checks passed" but the fleet is dead. HealthMonitor compounds the problem by retrying the same failing wake indefinitely.
- Evidence: First conductor start passed check-env, then failed 9 consecutive wake attempts (3 sprites x 3 retries) before HealthMonitor took over with the same failing strategy

### Finding: Empty-queue restart loop

- Severity: P2/medium
- Existing backlog item or new: **updated** — `backlog.d/004-loop-persistence.md` (added clean-exit backoff)
- Observed: bb-fixer and bb-polisher exit within ~4 min of launch when no open PRs exist. HealthMonitor detects exit, immediately relaunches. Cycle repeats indefinitely.
- Expected: HealthMonitor should distinguish "clean exit, no work" from "crashed loop" and back off relaunch when sprites repeatedly exit cleanly
- Why it matters: each restart cycle burns ~4 min of API tokens and sprite compute (context reading, skill loading, GitHub API calls) discovering there's nothing to do
- Evidence: bb-fixer: started 10:53:06, exited 10:57:11 (~4 min), relaunched 10:59:34, loop confirmed 11:01:52. bb-polisher: started 10:53:07, exited 10:59:41 (~6 min), relaunched 11:01:59.

### Finding: Codex model refresh timeout

- Severity: P3/low
- Existing backlog item or new: not filed (transient, single occurrence)
- Observed: bb-builder's Codex harness logged `failed to refresh available models: timeout waiting for child process to exit` at startup
- Expected: clean model resolution
- Why it matters: may cause model fallback behavior; worth watching for recurrence
- Evidence: bb-builder sprite logs, first line after loop start

## Backlog Actions

- **New**: `backlog.d/017-check-env-false-green.md` (P0/critical) — check-env false-green when sprite CLI keyring auth is expired
- **Updated**: `backlog.d/004-loop-persistence.md` (critical) — added clean-exit backoff to sequence and evidence section
- Priority changes: none

## Reflection

- What Bitterblossom did well:
  - Reconciler woke all 3 stopped Fly machines and provisioned them in 38s (once auth was fixed)
  - Health monitor correctly detected loop exits and relaunched sprites
  - Store events accurately recorded all health transitions
  - Spellbook bootstrap was fast and reliable (~8s per sprite)
  - Sprites made correct autonomous decisions: read context, checked for work, exited cleanly when none found

- What felt brittle:
  - check-env is a confidence gate that doesn't check what matters (live sprite reachability)
  - HealthMonitor has no concept of "no work available" — treats all loop exits as failures
  - The failed first start burned 9 wake attempts + multiple HealthMonitor recovery cycles before operator noticed
  - No notification when the fleet enters all-degraded state (relates to 014-notification-first-observability)

- What should be simpler next time:
  - check-env should either live-probe a sprite or verify the sprite CLI auth directly
  - Sprites exiting with "no work" should signal this to the conductor to avoid restart loops
  - `mix conductor start` should fail fast (not hand off to HealthMonitor) when ALL sprites fail reconciliation with the same error
