# Epic: operator dashboard for live runs, history, agents, triggers, and cost

Priority: P2 | Status: ready | Estimate: L

## Goal

Land the dashboard as an accountability surface over BB's read APIs: fleet,
live runs, run history, agents, triggers, cost vs budget, governance state, and
safe next actions. The dashboard must remain a drill-down surface, not a pane
that operators have to watch constantly.

## Oracle

- [ ] Dashboard data comes from `bb ... --json`-equivalent read helpers and
      `/api/*`; no dashboard-only truth.
- [ ] It shows live runs, stale/fresh classification, submissions/gates, agents,
      triggers, cost vs budget, governance state, DLQ/recovery, and notifications.
- [ ] It uses the Misty Step aesthetic kit or the fleet-approved visual baseline.
- [ ] It has usable empty, loading, error, auth-required, and no-data states.
- [ ] Read API routes are auth-gated and covered by curl/fixture tests.
- [ ] `./scripts/verify.sh` passes, and a browser or rendered screenshot check
      proves the dashboard is nonblank and readable.

## Children

- [ ] Reconcile the `bb-dashboard` branch against current `operator.html`.
- [ ] Fill read API gaps for agents, triggers, governance, and notification
      outbox.
- [ ] Cost/history charts from ledger data.
- [ ] Stale/recovery/DLQ drill-downs with safe next actions.
- [ ] Visual polish with the approved aesthetic baseline.

## Notes

The groom evidence says dashboard-first operations are refused. This epic is
therefore evidence-first: the dashboard must expose the same truth agents use
and make failure harder to miss, not replace notifications.
