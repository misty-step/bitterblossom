# Factory Audit Report — Run 5: API Key Auth Validation

## Summary

- Date: 2026-04-03 14:40–15:10 UTC
- Backlog item: Validation of 025 (auth isolation via API key)
- Fleet: bb-builder (Weaver), bb-fixer (Thorn), bb-polisher (Fern)
- Sprites dispatched: 3
- PRs opened: 0 new (2 existing: #835, #837)
- PR commits pushed: 1 (Fern pushed `391fd6a` to #837)
- Terminal state: All 3 sprites healthy, 2 running, zero auth failures

## Verdict: AUTH FIX WORKS. Factory produces real work.

This is the first factory run where sprites did actual productive work — reading backlogs, writing code, running tests, reviewing PRs, pushing commits. Zero `sprite_auth_failure` events across 30 minutes with 3 concurrent sprites. The auth contention blocker (025) is resolved.

## Timeline

| Time (UTC) | Event | Notes |
|------------|-------|-------|
| 14:40:25 | conductor start | 3 sprites loaded, OPENAI_API_KEY set |
| 14:40:28 | fleet reconciled | 3 healthy, 0 degraded |
| 14:40:49 | stale auth warnings | `refresh_token_reused` detected on old ChatGPT auth (expected, skipped) |
| 14:40:52 | spellbook bootstrapped | All 3 sprites |
| 14:40:55 | all loops started | bb-builder, bb-fixer, bb-polisher |
| 14:42:58 | loops confirmed | bb-builder, bb-polisher confirmed healthy |
| ~14:43 | Thorn exits cleanly | No broken PRs found — correct behavior |
| 14:47:16 | Thorn launch timeout | Health monitor detected exit, 3 ticks |
| 14:49:26 | Thorn recovered | Relaunched, second PR scan |
| ~14:50 | Weaver picks 003-muse-agent | Read backlog, chose high-priority item |
| ~14:55 | Weaver writes code | Modified workspace.ex, launcher.ex, tests |
| 14:58:04 | Weaver focused tests pass | 36 tests, 0 failures (including new Muse path) |
| ~15:02 | Weaver full suite | 219 tests, 1 flaky timeout (pre-existing) |
| ~15:05 | Fern pushes to #837 | Reverted backlog statuses (wrong — stale workspace) |
| 15:06:45 | Weaver creates checkpoint v15 | About to commit and push |
| 15:10 | audit stopped | All 3 healthy, zero auth failures |

## Health Monitor Observations

- Initial health: all 3 healthy/idle
- Transitions: Thorn exit → launch_timeout (3 ticks) → recovered. Expected for a sprite with no work.
- Stuck states: none
- Auth failures this run: **ZERO** (vs ~200 in overnight run)

## Findings

### Finding 1: Auth contention resolved

- Severity: **RESOLVED** (was critical)
- Evidence: Zero `sprite_auth_failure` events across 30 minutes, 3 concurrent sprites
- Before: `sprite_auth_failure` every 4-6 minutes, all sprites cycling on `refresh_token_reused`
- After: All sprites running independently with OPENAI_API_KEY, no token rotation contention

### Finding 2: Workspace staleness causes incorrect review judgments

- Severity: **P1**
- Existing backlog item: new
- Observed: Fern reviewed PR #837 (backlog status updates) but its sprite workspace was 2 commits behind master. It correctly observed that the code didn't reflect "done" claims — but only because it couldn't see PR #836's changes (which ARE merged).
- Expected: Sprites should pull latest master before reviewing PRs
- Why it matters: A polisher making confident but wrong decisions based on stale context will create churn (reverting correct work, filing incorrect review comments)
- Evidence: Fern's log item_40: "I've confirmed the relevant code path does not implement 020 or 024, and 025 is still using shared auth sync. The right fix is to make the backlog status reflect reality." → pushed revert commit.

### Finding 3: Weaver budget inefficiency with gpt-5.4-mini

- Severity: **P2**
- Existing backlog item: relates to 022 (model routing)
- Observed: Weaver spent ~15 minutes and 86+ log items reading context (CLAUDE.md, AGENTS.md, project.md, skill files, test files, all sprite AGENTS.md) before writing any code. The "Budget Discipline" instructions in Weaver's AGENTS.md were not followed.
- Expected: Weaver reads 1 backlog item + only files it needs to modify
- Why it matters: gpt-5.4-mini is cheaper but less efficient at instruction-following. It may spend its entire session budget on context gathering without producing code.
- Evidence: 43 items before first file_change event; project.md read despite AGENTS.md saying "Do NOT read project.md"

### Finding 4: Stale branch on builder workspace

- Severity: **P2**
- Existing backlog item: relates to workspace staleness (Finding 2)
- Observed: Weaver started on branch `weaver/017-check-env-false-green-2` from a previous run. Never created a fresh branch from origin/master despite AGENTS.md instruction "Always branch from current origin/master."
- Expected: Workspace reset to origin/master before dispatch
- Why it matters: Code changes may be committed on wrong branch, creating confusion

### Finding 5: Pre-existing test timeout under sprite load

- Severity: **P3**
- Existing backlog item: none (cosmetic)
- Observed: `cli_fleet_test.exs:601` (unknown subcommand test) timed out during full suite on sprite. Passes in isolation.
- Expected: Tests shouldn't be flaky under normal sprite load
- Why it matters: Can block sprite commits if they treat any test failure as blocking

## Backlog Actions

- New item needed: **Workspace freshness before dispatch** — `git fetch origin && git checkout origin/master` in workspace setup before launching agent loops. Prevents stale branch and stale context issues.
- Update 022 (model routing): add note about gpt-5.4-mini budget efficiency vs instruction-following tradeoff. Consider gpt-5.4 for builder role, mini for fixer/polisher.
- Update 025: mark as **done** (verified by this audit — zero auth failures, 3 concurrent sprites, 30 minutes)
- Consider: adding `@tag timeout: 90_000` to `cli_fleet_test.exs:601` to handle sprite-load flakiness

## Reflection

- **What Bitterblossom did well:**
  - Auth isolation works perfectly — zero contention, all sprites productive
  - Infrastructure is solid: health monitor, event logging, recovery all correct
  - Sprites exercise independent judgment (Fern verifying claims, Thorn correctly exiting)
  - Weaver wrote real code, ran tests, created checkpoint
  - Self-healing cycle visible: Thorn exit → detected → relaunched

- **What felt brittle:**
  - Workspace staleness is the #1 remaining issue — affects both builder and polisher accuracy
  - gpt-5.4-mini reads too much context, may exhaust budget before coding
  - No PR was opened in 30 minutes (Weaver close but not yet committed)

- **What should be simpler next time:**
  - `SPRITES_ORG` should be read from fleet.toml, not required as env var
  - Workspace should auto-reset to latest master on dispatch
  - Completed backlog items should be auto-retired when PRs merge
