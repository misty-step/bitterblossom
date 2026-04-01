# Factory Audit Report — Run 2

## Summary

- Date: 2026-04-01
- Backlog item: 003-muse-agent (picked autonomously by Weaver)
- Fleet: 3 sprites (bb-builder/Weaver, bb-fixer/Thorn, bb-polisher/Fern)
- Sprites dispatched: 3 (1 actually worked, 1 stuck, 1 restart-looping)
- PRs opened: 0 (Weaver's session ended before push)
- Terminal state: partial delivery — code written, tests green, not shipped

## Context

Run 2 followed PR #827 which fixed Weaver's AGENTS.md to read `backlog.d/` instead of `gh issue list`. The goal was to verify the factory could autonomously pick and build work.

## Timeline

| Time | Event | Notes |
|------|-------|-------|
| 14:12:48 | Conductor start | SPRITES_ORG=misty-step required |
| 14:12:52 | All 3 healthy (4.5s) | Sprites warm from run 1 |
| 14:12:58 | bb-fixer bootstrap failed | Broken symlink: debug → investigate |
| 14:12:59 | bb-polisher: "already has active loop" | Stale loop from run 1 |
| 14:13:00 | bb-builder: "already has active loop" | Stale loop from run 1 |
| 14:19:13 | All 3 launch timed out (3 probes) | Health monitor couldn't start new loops |
| 14:20 | Manual `sprite stop` on all 3 | Operator killed stale loops |
| 14:21:28 | bb-polisher recovered + relaunched | Fresh loop with updated AGENTS.md |
| 14:21:28 | bb-fixer bootstrap failed again | Same broken symlink |
| 14:23:37 | bb-polisher loop confirmed | Running, found no PRs |
| ~14:25 | bb-builder stale loop: working on 003-muse | Autonomously picked backlog item |
| ~14:28 | bb-builder: 34 tests pass, 0 failures | Implementation verified |
| ~14:30 | bb-builder session ended | Before git commit/push/PR |
| 14:31:23 | Conductor stopped | |

## Health Monitor Observations

- Stale loops from run 1 blocked new launches for all 3 sprites
- Health monitor correctly timed out after 3 probes (`:launching` → `:unhealthy`)
- After manual loop kill, recovery worked for bb-polisher
- bb-fixer stuck in bootstrap-fail → setup_incomplete loop
- bb-builder's stale loop was actually productive (doing real work)

## Findings

### Finding: Reconciler doesn't pass fleet.toml org to sprite operations

- Severity: P1/high
- Backlog: **new** — `backlog.d/018-reconciler-org-passthrough.md`
- Observed: fleet.toml says `org = "misty-step"` but reconciler fell through to sprite CLI default org `adminifi`
- Expected: sprite operations use the org from fleet.toml
- Root cause: `wake_opts`, `check_health`, `provision_and_verify` don't pass `sprite.org` to `Sprite.exec/3`
- Workaround: `export SPRITES_ORG=misty-step`

### Finding: Stale loops block new dispatch (confirmed)

- Severity: P0/critical (confirming 004-loop-persistence)
- Backlog: existing — `backlog.d/004-loop-persistence.md`
- Observed: all 3 sprites had active loops from run 1; conductor couldn't start new ones
- Expected: launcher preflight kills stale processes before starting new loops
- This is the exact gap described in 004's unchecked sequence items

### Finding: Broken spellbook symlink blocks bb-fixer

- Severity: P1/high
- Backlog: **new** — `backlog.d/019-spellbook-broken-symlink.md`
- Observed: `debug` skill renamed to `investigate` in spellbook, but old symlink persists on sprite
- Expected: bootstrap cleans up stale symlinks
- Impact: bb-fixer stuck in infinite bootstrap-fail loop, completely unable to launch

### Finding: Weaver autonomously delivered partial work

- Severity: positive finding
- Observed: Weaver (bb-builder, stale loop from run 1) autonomously:
  1. Read backlog.d/ items
  2. Picked highest-priority ready item: 003-muse-agent
  3. Created worktree `bitterblossom-muse`
  4. Modified fleet.toml, workspace.ex, launcher.ex, and 2 test files
  5. Ran `mix deps.get`, `mix compile`, `mix test` — all 34 tests passed
- Not shipped: Codex session ended before git commit/push/PR
- This proves the autonomous build loop works when the work source is correct

## Backlog Actions

- **New**: `backlog.d/018-reconciler-org-passthrough.md` (P1) — reconciler org fallback bug
- **New**: `backlog.d/019-spellbook-broken-symlink.md` (P1) — stale symlinks block bootstrap
- **Confirmed**: `backlog.d/004-loop-persistence.md` (critical) — stale loops block dispatch, proven by both run 1 and run 2

## Reflection

- What Bitterblossom did well:
  - **Weaver autonomously picked and implemented a backlog item** — first confirmed autonomous delivery in a factory audit
  - Health monitor correctly timed out stale launches
  - Recovery worked for bb-polisher after manual intervention
  - All infrastructure plumbing (wake, provision, bootstrap, dispatch) works when auth and state are clean

- What felt brittle:
  - Three separate auth/config issues (SPRITES_ORG, sprite CLI keyring, FLY_API_TOKEN false-green) — any one blocks the fleet
  - Stale sprite state (loops, symlinks) persists across conductor restarts and blocks recovery
  - bb-fixer was completely stuck — no self-healing possible for a broken symlink
  - Weaver's implementation was lost because the session ended before pushing

- What should be simpler next time:
  - `mix conductor start` should kill stale loops as preflight (004)
  - Reconciler should use fleet.toml org directly (018)
  - Bootstrap should be idempotent — clean broken symlinks (019)
  - Consider saving Weaver's work (commit but don't push) even if the session ends
