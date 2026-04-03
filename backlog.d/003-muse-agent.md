# Muse agent: post-completion reflection + backlog management

Priority: high
Status: ready
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

## Sequence
- [ ] Create `sprites/muse/AGENTS.md` with reflection loop definition
- [ ] Create `sprites/muse/CLAUDE.md` with Muse identity and constraints
- [ ] Add `bb-muse` to fleet.toml (role: `triage`)
- [ ] Muse AGENTS.md: poll recently merged PRs, read diff + backlog item, run /reflect
- [ ] Muse AGENTS.md: update backlog.d/ items based on findings, commit and push
- [ ] Test: run Muse after a manual merge, verify it produces backlog updates

## Oracle
- [ ] After a PR merges, Muse reads the diff and the source backlog item
- [ ] Muse produces at least one backlog action (update, consolidate, or create)
- [ ] Retro notes are written to `.groom/retro/`
- [ ] Muse does NOT implement code — it observes and recommends
