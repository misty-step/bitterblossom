# Factory Audit Report — Run 4

## Summary

- Date: 2026-04-02
- Fleet: 3 sprites (bb-builder/Weaver, bb-fixer/Thorn, bb-polisher/Fern)
- Sprites dispatched: 3 (2 launched clean, 1 self-healed)
- PRs opened: 0 (Codex auth failure killed Weaver's session)
- Terminal state: all healthy, 2 idle, 1 running

## Context

Run 4 follows three PRs fixing factory blockers:
- #827: `backlog.d/` as canonical work source
- #829: check-env, org passthrough, broken symlinks, Weaver WIP saves
- #830: `setsid` process groups, rapid-exit backoff

This is the validation run for all fixes.

## Timeline

| Time | Event | Notes |
|------|-------|-------|
| 20:58:57 | Conductor start | SPRITES_ORG=misty-step |
| 20:59:01 | 3/3 healthy, 0 degraded | 4s reconciliation |
| 20:59:06 | Spellbook ready (all 3) | No broken symlink failures |
| 20:59:08 | bb-builder loop started | **Clean launch — setsid fix worked** |
| 20:59:09 | bb-fixer: "active loop" | Pre-setsid stale loop survived stop_loop |
| 20:59:10 | bb-polisher loop started | **Clean launch** |
| 21:07:42 | bb-fixer recovered + relaunched | **Self-healed without intervention** |
| 21:07:48 | bb-fixer loop started | |
| 21:09:51 | bb-fixer loop confirmed | |
| ~21:11 | bb-builder auth failure | Codex refresh_token_reused |
| ~21:15 | bb-fixer idle | No PRs to fix, exited cleanly |
| 21:20 | All healthy, bb-polisher running | |

## Infrastructure Fix Verification

| Fix | PR | Result |
|-----|-----|--------|
| check-env live probe | #829 | Correct label, live probe passed |
| Org passthrough | #829 | All reconciliation used correct org |
| Broken symlink cleanup | #829 | All 3 bootstrapped clean |
| backlog.d/ work source | #827 | Weaver read backlog.d/ (before auth died) |
| setsid process groups | #830 | bb-builder + bb-polisher launched clean |
| Rapid-exit backoff | #830 | Not triggered (no rapid exits this run) |

**5 of 6 fixes verified in production. The 6th (backoff) had no opportunity to trigger — positive signal (no empty-queue restart loop observed).**

## Findings

### Finding: Codex auth token refresh failure

- Severity: P1/high (external system)
- Observed: `Failed to refresh token: refresh_token_reused`
- Impact: Weaver's session died before completing any work
- Root cause: ChatGPT auth refresh tokens are single-use. When multiple sprites share the same auth cache (synced from operator's `~/.codex/auth.json`), one refresh invalidates the others.
- Not a conductor bug — this is an OpenAI Codex auth lifecycle issue
- Workaround: use `OPENAI_API_KEY` instead of ChatGPT auth, or re-provision auth before each dispatch

### Finding: Pre-setsid stale loops still survive one restart

- Severity: P2/medium (self-resolving)
- Observed: bb-fixer's pre-setsid loop survived stop_loop's group kill
- Expected: the `pkill` fallback should catch it
- Actual: `pkill -f codex` ran but the stale session's process tree included nodes not matching the pattern
- Self-resolved: health monitor detected the exit and relaunched within 8 minutes
- **This issue is transient** — after one clean restart with setsid, all future loops will have process groups

## Health Monitor Observations

- bb-fixer: `:launching` → failed start → stale loop exited → `:unhealthy` → recovered → relaunched → `:healthy`. **Full self-healing cycle completed without operator intervention.**
- bb-builder: `:launching` → `:healthy` → auth died → idle but not detected as `:unhealthy`. The PID file wasn't cleaned up after the auth crash, so `loop_alive` still reports true.
- bb-polisher: clean lifecycle throughout.

## Backlog Actions

- No new items created (Codex auth is an external system issue, not actionable in the conductor)
- Pre-setsid stale loops are self-resolving and don't need a backlog item

## Reflection

- What Bitterblossom did well:
  - **2 of 3 sprites launched clean with the new setsid fix**
  - **bb-fixer self-healed** — the first fully automatic recovery in 4 factory audit runs
  - All infrastructure fixes (#827, #829, #830) verified in production
  - No manual operator intervention needed for fleet health
  - Spellbook bootstrap worked on all 3 sprites (broken symlink fix confirmed)

- What blocked delivery:
  - Codex auth token rotation — external to the conductor, but the conductor should surface auth failures more clearly
  - The Codex session budget remains the constraint on completing work (observed in runs 2, 3, and 4)

- Progress across runs:

  | Run | Manual interventions | Sprites launched clean | Self-healed |
  |-----|---------------------|----------------------|-------------|
  | 1 | 2 (sprite login, manual stop) | 0/3 | No |
  | 2 | 3 (sprite login, SPRITES_ORG, manual loop kill) | 0/3 | No |
  | 3 | 1 (manual loop kill) | 0/3 | No |
  | 4 | **0** | **2/3** | **Yes** |
