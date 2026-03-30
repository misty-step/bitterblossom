# Pivot docs and project.md to thin-wrapper vision

Priority: high
Status: ready
Estimate: S

## Goal
Update project.md, CLAUDE.md, README.md, and root docs to reflect the architectural pivot: Bitterblossom is a thin, opinionated CLI wrapper around the Sprites platform for declarative sprite management. Not a cybernetic governor. Not a run-centric control plane.

## Non-Goals
- Rewrite every doc in the repo (focus on the load-bearing ones agents read)
- Document the dashboard UI (that comes when it's built)

## Sequence
- [ ] Rewrite `project.md` — new vision statement, remove "cybernetic governor" / "run-centric control plane" / "leases" / "governance" language. New framing: thin CLI for sprite CRUD, declarative fleet management, Phoenix dashboard for observability
- [ ] Update `CLAUDE.md` — align architecture section, operating model, domain glossary. Remove references to runs, leases, governance gates, review council
- [ ] Update `README.md` — align with actual CLI surface and thin-wrapper vision
- [ ] Update or archive `docs/CONDUCTOR.md` — already flagged stale in audit #630
- [ ] Review ADR-004 — add a "superseded by" note if the thin-wrapper pivot changes its assumptions
- [ ] Remove stale domain glossary terms: Run, Lease, Review Council, Variant, Profile (if not used)

## Oracle
- [ ] `project.md` describes Bitterblossom as a thin CLI wrapper, not a control plane
- [ ] `grep -r "cybernetic governor" .` returns zero hits in project docs
- [ ] `grep -r "run-centric" .` returns zero hits in project docs
- [ ] CLAUDE.md domain glossary matches the actual codebase after 012 lands
- [ ] README.md quick-start instructions work from a fresh checkout

## Notes
Subsumes and replaces backlog item 007 (reconcile root docs). Item 007 was scoped for the old vision — this is broader.

This should land alongside or immediately after 012 (kill orchestration layer) so docs and code stay in sync.
