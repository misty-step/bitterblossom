# Add unattended-loop safety guardrails before expanding autonomous volume

Priority: P1 · Status: ready · Estimate: L

## Goal

Make recurring and webhook-triggered BB loops fail loudly, halt safely, and avoid billing or mutation incidents before Canary and backlog-chewer volume grows.

## Oracle

- [ ] A global pause/resume path exists for reflex dispatch, separate from per-task parking, with reason recorded and visible in `status --json`.
- [ ] Ingress enforces a maximum request body size and tests oversized webhook rejection without ledger growth.
- [ ] Cron catch-up is bounded by a configured max fires per tick or an explicit collapse policy; skipped/collapsed fires are recorded visibly.
- [ ] Notification delivery has a durable outbox or an explicitly smaller first slice that records failed notifications and surfaces them in status.
- [ ] Running attempts expose heartbeat/generation or last-progress evidence sufficient for stale detection without guessing.
- [ ] Budget accounting includes in-flight/reserved spend or documents a conservative cap policy for high-volume reflex tasks.
- [ ] `./scripts/verify.sh` passes.

## Verification System

- Claim: unattended loops cannot silently spin, flood, or fail invisibly under common webhook/cron/notification/recovery failures.
- Falsifier: repeated events enqueue unbounded work; notification failure is only stderr; paused plane still dispatches reflex runs; stale executing work has no operator-visible signal.
- Driver: dev plane storm drills for webhook body cap, cron catch-up, notification failure, pause/resume, and stale run simulation.
- Grader: status/API shows blocked/paused/outbox/stale states; no unbounded ledger growth; safe next action is machine-readable.
- Evidence packet: drill transcripts and status JSON snapshots.
- Cadence: before increasing Canary/backlog-chewer allowlists or authority levels.

## Rollout Gate Metrics

These guardrails are prerequisites for expanding autonomous volume. A workflow may not move from read/report-only into write authority unless status/API can show:

- global pause state and reason;
- per-task active runs, duplicate-suppression key, and max active concurrency;
- last progress heartbeat for running attempts and stale-run safe next action;
- notification/outbox failures;
- in-flight or reserved spend, not just terminal cost;
- cron/webhook collapse/skipped event counts;
- active authority level for each autonomous task family.

Promotion trigger: before any new autonomous repo/service allowlist expansion, run the guardrail drill and attach status JSON snapshots to the promotion issue.

## Notes

Why: previous hardening made the plane much safer, but the 2026-06-29 groom identified unsupervised-volume guardrails as a prerequisite for broader autonomous loops. This ticket is mechanism only: pause, caps, outbox/status, heartbeat, budget reservation. It must not add workflow judgment to the spine.

Related: 051 recovery/probe determinism and 072 observability. Keep this ticket focused on loop containment and noisy failure.
