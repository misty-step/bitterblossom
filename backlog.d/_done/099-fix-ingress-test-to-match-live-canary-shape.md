# Make the ingress test exercise the shape canary actually sends

Priority: P0 · Status: done · Estimate: S

## Goal
Bitterblossom's ingress test asserts intake of the REAL canary incident payload with a top-level `incident` object, so the test proves the live seam instead of an aspirational one.

## Oracle
- [x] `tests/ingress.rs:134` (the canary webhook case) uses a payload with top-level `incident` matching canary's live emitter (`canary-store/src/incidents.rs:505` shape), not `subject`.
- [x] The test drives the full path: HMAC-signed request → `handle_webhook` (`src/ingress.rs:26`) → task.toml routing (`plane/tasks/canary-triage/task.toml` pointer `/incident/service == "canary"`) → accepted (not filtered).
- [x] A companion negative test proves a `subject`-shaped payload is filtered with HTTP 200 — documenting the silent-drop failure mode explicitly.

## Notes
Coordinate with canary refill ticket 080 (pins the emitter shape on its side). The two tests together turn a silent cross-repo trap into a loud one. The future `subject`+`schema_version` migration stays a daylight, lockstep change.
**Why:** 2026-07-01 composition seam audit, Seam 2 — BB's own test currently validates a payload shape that no producer sends; prod works by coincidence of the task.toml pointer matching the emitter.

## Delivery Notes

Delivered 2026-07-02 in `tests/ingress.rs`.

- The accepted Canary ingress test now uses BB's pinned `tests/fixtures/contracts/canary.incident_event.v1.valid.json`, which carries the live emitter's top-level `incident` object (`/incident/service == "canary"`). The live Canary source currently still includes `schema_version`; the routing regression here was the missing `incident` object, not the schema-version field.
- The temporary test plane now mirrors the production `canary-triage` webhook route and the `/event` plus `/incident/service` filters.
- Added `webhook_filters_subject_only_canary_payload_without_a_run`, proving the old subject-only shape is acknowledged as filtered with HTTP 200 and creates no run or ingress ledger event.

Proof:

- `cargo test --test ingress canary -- --nocapture`
