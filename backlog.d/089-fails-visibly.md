# Epic: fails visibly before unattended volume grows

Priority: P0 | Status: ready | Estimate: XL

## Goal

Make "an unattended agent that fails visibly" a hard runtime invariant, not a
dashboard fact an operator has to remember to poll. Every non-terminal run and
submission must have a freshness contract, an escalation path, and a bounded
next action when that contract is breached.

This epic folds in the remaining hung-arm/stale-detection, submission-storm
timeout, notification-outbox, and self-drill work from the groom report. It does
not close existing tickets automatically; it names the consolidation target for
implementation PRs.

## Oracle

- [ ] Each run state and attempt phase has a documented freshness contract:
      threshold, owner, safe next action, and notification severity.
- [x] `stale_executing`, stale `awaiting_recovery`, and stuck submission arms
      emit durable notification-outbox rows with retry, ack, and visible status.
- [x] Notification delivery is no longer fire-and-forget curl; failures survive
      process restart and are retried or explicitly acknowledged.
- [ ] Serve-mode dispatch writes meaningful progress while execute is active or
      records why the harness cannot emit heartbeats.
- [x] Submission storms have a bounded arm timeout and quorum/escalation policy
      in config; a hung member cannot leave the gate pending forever.
- [ ] An attention-debt brake aggregates open DLQ, parked tasks, stale runs, and
      awaiting recovery, and can refuse new reflex admissions while dispatch
      remains operator-controlled.
- [ ] A weekly self-drill chaos reflex deliberately creates a controlled stale or
      failed run and verifies the expected escalation trail.
- [ ] `bb status --json`, `/api/status`, and `bb runs show --json` expose the
      freshness/outbox/escalation state needed by humans and agents.
- [ ] `./scripts/verify.sh` passes, including fixture drills for stale executing,
      stuck gate member, notify retry, and attention-debt admission refusal.

## Children

- [ ] Freshness-contract table in docs plus status JSON fields for every
      non-terminal state.
- [x] Durable notification outbox: schema, retry worker, ack CLI, and status/API
      projection.
- [x] Serve-mode heartbeat/watchdog for executing attempts, respecting the
      no-blind-replay side-effect boundary.
- [x] Submission arm timeout and quorum/escalation policy in `[gate]`.
- [ ] Attention-debt brake for reflex ingress.
- [ ] Self-drill chaos reflex task, report artifact, and failure notification.
- [ ] Backlog consolidation notes for 051, 083, 085, 087, and any storm-timeout
      follow-up produced during implementation.

## Notes

Groom evidence: PR #867 had zero successful webhook reviews for ten days and was
found by a sweep, not by the plane. PRs #873 and #874 added progress visibility,
pause/resume, ingress caps, cron bounded catch-up, notification-failure recording,
and reserved-spend status. This epic starts from that shipped baseline and closes
the remaining "visibility is not escalation" gap.
