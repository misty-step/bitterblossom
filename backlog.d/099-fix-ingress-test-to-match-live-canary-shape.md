# Make the ingress test exercise the shape canary actually sends

Priority: P0 · Status: ready · Estimate: S

## Goal
Bitterblossom's ingress test asserts intake of the REAL canary incident payload (top-level key `incident`, no `schema_version`), so the test proves the live seam instead of an aspirational one.

## Oracle
- [ ] `tests/ingress.rs:134` (the canary webhook case) uses a payload with top-level `incident` matching canary's live emitter (`canary-store/src/incidents.rs:505` shape), not `subject`.
- [ ] The test drives the full path: HMAC-signed request → `handle_webhook` (`src/ingress.rs:26`) → task.toml routing (`plane/tasks/canary-triage/task.toml` pointer `/incident/service == "canary"`) → accepted (not filtered).
- [ ] A companion negative test proves a `subject`-shaped payload is filtered with HTTP 200 — documenting the silent-drop failure mode explicitly.

## Notes
Coordinate with canary refill ticket 080 (pins the emitter shape on its side). The two tests together turn a silent cross-repo trap into a loud one. The future `subject`+`schema_version` migration stays a daylight, lockstep change.
**Why:** 2026-07-01 composition seam audit, Seam 2 — BB's own test currently validates a payload shape that no producer sends; prod works by coincidence of the task.toml pointer matching the emitter.
