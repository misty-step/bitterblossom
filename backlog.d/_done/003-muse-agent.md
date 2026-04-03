# Muse agent: post-completion reflection + backlog management

Priority: high
Status: done
Estimate: M

## Goal
After any sprite completes work (PR merged, issue closed), Muse runs `/reflect` on the completed work and updates the backlog:
- Consolidate backlog items that are now redundant
- Add new items discovered during implementation
- Reprioritize items based on new evidence
- Record learnings in `.groom/retro/<item>.md`

This is the feedback loop that makes the factory self-improving.

## Design
Muse is dispatched after each merge event (detected by the conductor or by polling recently merged PRs). It reads:
- The merged PR diff and comments
- The backlog.d/ item that was worked on
- Store events from the run
- Sprite logs

Then produces:
- Updated backlog items (priority changes, scope refinements)
- New backlog items for problems discovered during implementation
- Retro notes in `.groom/retro/`

## Oracle
- [x] After a PR merges, Muse reads the diff and the source backlog item
- [x] Muse produces at least one backlog action (update, consolidate, or create)
- [x] Retro notes are written to `.groom/retro/`
- [x] Muse does NOT implement code — it observes and recommends

## What Was Built
- `sprites/muse/AGENTS.md` defines Muse's reflection loop: scan merged PRs without retro notes, inspect PR metadata, store events, sprite logs, and git history, then update `backlog.d/` plus `.groom/retro/`.
- `sprites/muse/CLAUDE.md` defines the observer-only persona and constrains Muse to reflection and backlog synthesis.
- `fleet.toml` declares `bb-muse` with role `triage`.
- Conductor role mapping now routes `:triage` sprites to the Muse persona and prompt in workspace sync and launcher dispatch.
- Coverage exists in fleet loading, workspace persona selection, and launcher prompt wiring for the Muse role.

## Verification
- [x] `fc7e3e1` created the Muse persona files and archived earlier completed backlog work.
- [x] `c762368` wired the `:triage` role through `fleet.toml`, loader, workspace persona sync, launcher prompt construction, and tests.
- [x] `conductor/test/conductor/fleet/loader_test.exs`
- [x] `conductor/test/conductor/workspace_test.exs`
- [x] `conductor/test/conductor/launcher_test.exs`

## Notes
- The remaining work after `c762368` was backlog hygiene: this item had shipped but was still marked `ready`, which caused Weaver to keep selecting it as active work.
