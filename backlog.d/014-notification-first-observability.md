# Notification-first observability

Priority: medium
Status: ready
Estimate: S

## Goal
Add a notification channel so the operator gets pinged when sprites need attention, instead of watching the dashboard. Dashboard stays as drill-down. Industry consensus (Osmani, Composio): notifications are the primary interface for autonomous agent fleets.

## Non-Goals
- Replace the Phoenix dashboard (it's the drill-down surface)
- Build a full alerting/paging system
- Notification fatigue — only actionable events, not heartbeats

## Sequence
- [ ] Define notification-worthy events: sprite_degraded, sprite_crashed, fleet_unhealthy, sprite_recovered (recovery is useful context), bootstrap_failed
- [ ] Add `Conductor.Notifier` module — subscribes to `Conductor.PubSub`, filters for notification-worthy events, dispatches to configured channel
- [ ] Implement webhook channel: POST JSON payload to configured URL (covers Slack incoming webhooks, Discord, generic)
- [ ] Add `[notifications]` section to fleet.toml: `webhook_url`, `events` (list of event types to notify on)
- [ ] Wire `HealthMonitor` events through PubSub (may already be wired — verify)
- [ ] Test: degrade a sprite, verify webhook fires

## Oracle
- [ ] fleet.toml supports `[notifications]` config with `webhook_url`
- [ ] When a sprite goes unhealthy, a POST is sent to the configured webhook URL within 60 seconds
- [ ] When a sprite recovers, a POST is sent
- [ ] Dashboard still works independently of notifications
- [ ] No notifications fire for routine health checks (only state transitions)
- [ ] `mix test` passes (with webhook stubbed)

## Notes
PubSub infrastructure already exists (`Conductor.PubSub` via Phoenix). HealthMonitor already detects state transitions. This is plumbing the last mile: PubSub → filter → HTTP POST.

~50-100 LOC for the Notifier module. Keep it dead simple — one GenServer subscribing to PubSub, pattern-matching on event types, POSTing JSON.
