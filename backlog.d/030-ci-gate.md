# Add a CI gate so master cannot break silently

Priority: P1
Status: ready
Estimate: S

## Goal
Every push and PR to master runs the repo's verification (`mix format
--check-formatted`, `mix compile --warnings-as-errors`, `mix test`) in CI.

## Why now
`.github/workflows/` is empty. The factory's whole premise is autonomous
agents merging code, yet nothing machine-enforced stands between an agent
branch and a broken master — Thorn's "fix failing CI" role currently has no
CI to read. CLAUDE.md's edit-discipline section documents compile/test
thrashing as the top failure mode; a gate turns that doctrine into
enforcement.

## Oracle
- [ ] A workflow exists that runs format check, compile with
      warnings-as-errors, and `mix test` for `conductor/` on push and PR
- [ ] A deliberately broken test on a branch produces a red check
- [ ] The gate passes on current master

## Notes
Keep it one small workflow with dependency caching; no matrix, no release
plumbing. Complements 008 (local pre-commit guardrails) — this is the
server-side backstop.
