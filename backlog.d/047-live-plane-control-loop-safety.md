# Harden the live plane's control-loop safety before adding more reflex workloads

Priority: P0 · Status: ready · Estimate: L

## Goal

Make `bb serve` safe to leave running under real reflex load by removing
operator-token URL exposure, preventing in-flight task starvation, and bounding
notification fan-out.

## Oracle

- [ ] Read API/operator-view auth no longer requires putting `BB_API_TOKEN` in
      a URL; tests and docs cover the browser/operator path and bearer-header
      path.
- [ ] Dispatch in-flight bookkeeping cannot strand a task if a run worker
      panics or returns before normal cleanup; a test proves the next pending
      run for the task can drain.
- [ ] Notifications have bounded concurrency, backpressure, or synchronous
      accounting; a notification storm test proves the process does not spawn
      unbounded curl waiters.
- [ ] Live QA exercises `bb serve` read API/HTML with and without
      `BB_API_TOKEN`, plus `./scripts/verify.sh`.

## Notes

Evidence from the 2026-06-13 groom:

- `src/serve.rs:103-140` keeps an in-memory `in_flight` task set and removes
  entries after `run_one`; a worker panic before removal can starve that task
  until process restart.
- `src/serve.rs:209-220` accepts `?token=` for read API/operator auth, and
  `docs/spine.md:245-247` documents that browser path. Query tokens leak
  through browser history, logs, screenshots, and referrers.
- `src/notify.rs:16-48` spawns `curl`, then spawns one waiting thread per
  notification with no concurrency bound.
- The current live plane has a parked `security` verdict task from a cost
  breach; reflex expansion should wait until the control loop's own safety
  boundaries are boring.

Keep the spine generic. This is dispatch/serve/notify hardening, not
review-workload logic.

## Mega groom disposition 2026-06-13

This remains the first concrete child of
`backlog.d/050-event-plane-hardening-before-growth.md`. Do not treat 047 as
the whole strategy: the mega-groom widened it into a hardening-before-growth
epic that also includes CLI/API contract drift, the first ledger-native health
surface, and live API/HTML QA.
