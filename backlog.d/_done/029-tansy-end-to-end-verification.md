# Verify the Tansy lane end-to-end against a live or replayed Canary incident

Priority: P1
Status: abandoned
Estimate: M

## Goal
Prove the active product — Tansy claims a Canary incident, investigates,
repairs the target repo, and verifies recovery — works today, with fresh
evidence on disk.

## Why now
The direction lock says Tansy is the one active lane, but the newest
verification evidence is `.evidence/feat/tansy-canary-responder/2026-04-09/`
— a two-month-old code-review verdict, not a live-incident run. Since then
the sprite launch path, log lookup, and canary catalog have all been patched
(70ecae7, 3c43932). Nothing on disk proves the loop survives those changes.

## Oracle
- [ ] A real or replayed Canary incident is dispatched to bb-tansy via
      `mix conductor start --fleet ../fleet.toml` (or a documented
      single-incident harness)
- [ ] Tansy claims the incident, annotates it in Canary, and produces a fix
      or a structured no-action verdict in the target repo
- [ ] Recovery verification runs and its result is recorded
- [ ] Evidence (timeline, store events, verdict) lands under `.evidence/`
      with a current date

## Notes
If a replay harness doesn't exist, building a minimal one is in scope —
"wait for production to break" is not a verification strategy. Absorbs the
live-verification intent of archived 011 (Tansy already implements its
cursor/dedup/annotation discipline).
