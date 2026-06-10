# Notification-first observability

Priority: P2
Status: done
Estimate: S

> Groom 2026-06-10: rewritten for v3. The original body was Elixir-conductor
> plumbing (Conductor.Notifier, Phoenix PubSub); the principle survives the
> teardown — notifications are the operator interface for an unattended
> plane, the dashboard is drill-down.

## Goal
The operator gets pinged when the event plane needs attention — run
dead-lettered, budget exceeded, workload repeatedly failing, recovery
confirmed — instead of watching anything.

## Oracle
- [x] A configurable webhook channel (covers Slack/Discord/ntfy) receives a
      POST on: run dead-lettered, budget threshold crossed, run orphaned
      at boot — src/notify.rs, fired from dispatch (dead letter, budget
      park/block) and serve boot recovery; tests/budgets.rs asserts the
      dead-letter and both budget events through the real notify path
- [x] State-transition events only — no heartbeats, no per-run noise
      (success/running emit nothing)
- [x] Notification config lives with the plane config
      (plane.toml [notify].webhook_url), not per-task boilerplate

## Notes
Depends on the 031 spine (ledger + run state machine emit the events).
Keep it one small module: filter ledger transitions, POST JSON.
