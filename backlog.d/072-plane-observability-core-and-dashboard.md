# Make the plane observable: core read surface + thin dashboard

Priority: P1 | Status: ready | Estimate: L

## Goal

Make "what's configured, what's running, what we've run, and how healthy it
is" answerable from one core read surface — CLI-first for the agent (every
datum has a `bb … --json`), HTTP-mirrored for tools — and put a thin static
dashboard on top for humans, with no workload logic in the spine.

## Oracle

- [ ] Every dashboard datum is reachable from both `bb <verb> --json` (primary,
      agent-facing) and a `GET /api/*` route (mirror) — no HTML-only data path.
- [ ] The four buckets are complete (see Children): Configured, Running,
      History, Health.
- [ ] The dashboard is a static asset served by `bb serve` (the existing
      `operator.html` shell), consuming only the JSON API; net `src/` cost is
      read-mirrors only (~≤30 LOC, mechanism).
- [ ] `bb serve` + curl every `/api/*` route with and without `BB_API_TOKEN`
      passes (extends the existing Read-API QA recipe); the control-loop drill
      covers the new routes.
- [ ] `./scripts/verify.sh` passes.

## Children

1. **Core read-surface gaps** (P1, ready, S). Fill the four MISSING/partial
   reads found in the surface audit: (a) trigger detail in `/api/tasks`
   (kind/route/cron-schedule, not just a count); (b) `GET /api/leases` +
   `Ledger::list_leases()` (in-flight host leases); (c) trigger/ingress
   activity (last cron/webhook fired, over the already-populated
   `ingress_events` table); (d) run-level tokens + a `GET /api/export` mirror of
   `bb runs export`. Each is a near-verbatim mirror of existing query/CLI code.
2. **Read-API shape consistency** (P2, ready, S) — **see backlog 066** (the
   `bb gate` top-level `rev` vs `/api/submissions` nested `submission.rev`
   bug). Kept standalone; this epic depends on it for correct summarization.
3. **Thin static dashboard** (P2, ready, M). Finish `operator.html`
   (`src/serve.rs` `GET /`) into a drill-down dashboard with Configured /
   Running / History / Health views. Notification-first, drill-down — not a
   pane of glass to watch (project.md:83-84). No new workload logic.

## Verification System

- Claim: the plane's full operational state is visible from the CLI (agent) and
  a thin dashboard (human) over one API.
- Falsifier: a datum the dashboard shows that no `bb --json`/`/api` returns; new
  workload judgment in `src/`; or the dashboard reading the DB directly.
- Driver: `bb serve` on the dev plane with seeded runs/parks/DLQ/leases; curl
  each route; load `/` in a browser.
- Grader: each bucket's data present in CLI JSON and API JSON; auth-gating
  honored; `bb check` + verify.sh green; LOC budget respected.
- Evidence packet: curl transcripts per route (token on/off) + a dashboard
  screenshot, under the repo evidence path.
- Cadence: the Read-API/HTML QA recipe in CLAUDE.md, extended to the new routes.

## Notes

Vision-backed and under-built, not greenfield. project.md North Star is "define
a task … and then **watch it**"; `project.md:63` names "metrics routes and a
dashboard for humans"; `:83-84` fixes the shape — "notification-first … the
dashboard is drill-down, not a pane of glass to watch"; quality bar `:108-109`
already requires CLI visibility of cost/burn/retries/DLQ/queue. The surface
audit (groom 2026-06-17) found the read API + `operator.html` shell **already
exist** — this is *finishing* it (4 thin reads + the HTML), hence the tiny
budget. CLI stays the first-class interface for the primary user (the agent);
the dashboard is one more human-oriented layer. Consolidates 066 as child 2 by
reference (066 stays its own pickup-able ticket).
