# BB self-reports health and errors to Canary

Priority: P0 | Status: ready | Estimate: S

## Goal

Bitterblossom reports its own liveness and runtime errors to the live Canary
instance (`canary-obs.fly.dev`), closing the dogfooding gap where BB triages
other repos' incidents but reports none of its own crashes/staleness.

## Oracle

- [ ] Periodic TTL check-in to monitor `bb-plane` on the live Canary instance.
- [ ] Runtime errors (plane-load failures, dispatch failures, cron failures,
      watchdog escalations, thread panics) reported as Canary error events for
      service `bb-plane`.
- [ ] `CANARY_ENDPOINT` and `CANARY_INGEST_KEY` read from env; module is a
      silent no-op when either is unset — no regressions in local dev or CI.
- [ ] No new crate dependencies; outbound HTTP follows the same `curl`-via-stdin
      subprocess pattern as `src/notify.rs`.
- [ ] `./scripts/verify.sh` passes.
- [ ] `bb-plane` TTL monitor and scoped ingest key exist on the live Canary
      instance (created idempotently by the implementer).

## Design

New module `src/canary.rs`:
- `canary::enabled()` — true when env vars are present
- `canary::check_in()` — POST `/api/v1/check-ins` with `monitor=bb-plane`, `status=alive`, `ttl_ms=120000`
- `canary::report_error(class, message, stack)` — POST `/api/v1/errors` with `service=bb-plane`
- `canary::start_health_loop()` — spawns `bb-canary-health` thread, 60s interval

Wired into `serve.rs`:
- Health loop spawned alongside cron/dispatch/notify/watchdog threads
- Boot check-in sent before http_loop

Wired into error paths:
- Plane load failures in `serve::serve()`
- Recovery failures
- Dispatch/cron failures
- Watchdog scan failures

Service name `bb-plane` is intentionally distinct from the Fly app name
`bitterblossom-plane` — Canary service names match the logical service, not
the deploy target.

## Notes

Flagged by the 2026-07-02 fleet assessment: "BB's own crashes/staleness to the
incident system" — nothing pipes BB's failures to the incident system it triages
other repos' incidents with.