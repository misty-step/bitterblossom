# Issue 779 Dashboard Operator Visibility Plan

> Scope issue: #779

## Goal

Turn the existing LiveView run table into the operator dashboard the conductor actually needs: one page that exposes fleet health, phase worker activity, governor cooldowns, recent events, and recent run history without adding new persistence or management controls.

## Product Spec

### Problem

The operator still needs shell access to answer basic questions about the factory:

- which sprites are healthy or degraded
- whether Thorn or Fern are actively working or backing off
- which issues are currently in cooldown and why
- what the factory has done recently across runs and phase workers

The existing dashboard only shows a recent-runs table, and it is disabled by default. That leaves the repo with a LiveView surface that exists architecturally but does not yet satisfy the operator visibility contract described in the issue and ADR.

### Intent Contract

- Intent: ship a single operator dashboard that exposes truthful runtime visibility from existing status calls and store queries.
- Success Conditions: `mix conductor start` brings up the dashboard automatically, the page shows fleet health, phase worker status, governor cooldowns, recent events, historical run summaries, and the existing run table, and these panels refresh from PubSub-backed runtime updates.
- Hard Boundaries: keep this as one dashboard page, reuse existing runtime state instead of adding new tables, and avoid management controls such as pause/resume or force actions.
- Non-Goals: build a separate admin console, add new durable metrics storage, or redesign the conductor process model to satisfy the UI.

## Technical Design

### Approach

1. Enrich the existing status surfaces just enough for visibility:
   - `HealthMonitor.status/0` returns per-sprite role, current status, last probe time, consecutive failures, interval, and last fleet check time.
   - `Fixer.status/0` and `Polisher.status/0` return poll interval plus recent dispatch/completion timestamps in addition to health and in-flight work.
2. Make dashboard refresh truthful for runtime events by broadcasting on the dashboard PubSub topic when store events are recorded, not only when runs change.
3. Expand `DashboardLive` into a multi-panel page that assembles:
   - fleet status from `HealthMonitor.status/0`
   - phase worker status from `Fixer.status/0` and `Polisher.status/0`
   - governor cooldowns by combining open issues with `Store.issue_failure_streak/2` and the orchestrator cooldown formula
   - recent events from `Store.list_all_events/1`
   - recent run history from `Store.list_runs/1`
4. Enable the dashboard by default in application boot and ensure the endpoint is configured with a usable secret/server setup when the dashboard lane is active.

### Files to Modify

- `conductor/lib/conductor/application.ex`
- `conductor/lib/conductor/store.ex`
- `conductor/lib/conductor/fleet/health_monitor.ex`
- `conductor/lib/conductor/fixer.ex`
- `conductor/lib/conductor/polisher.ex`
- `conductor/lib/conductor/web/dashboard_live.ex`
- `conductor/config/config.exs`
- `conductor/test/conductor/dashboard_live_test.exs`
- `conductor/test/conductor/fleet/health_monitor_test.exs`
- `conductor/test/conductor/fixer_test.exs`
- `conductor/test/conductor/polisher_test.exs`

### Implementation Sequence

1. Write or extend focused tests for the richer health/worker status contracts and the new dashboard sections.
2. Add the minimal runtime metadata needed for the dashboard surfaces.
3. Rebuild the LiveView around a single dashboard snapshot with sectioned panels and event-source filtering.
4. Enable auto-start and run focused tests, then the full conductor suite if the slice is clean.

### Risks & Mitigations

- Risk: the dashboard reaches into too many runtime details and becomes a shallow pass-through view.
  Mitigation: keep the runtime-specific assembly in status/query helpers and have the LiveView render a single snapshot.
- Risk: live GitHub issue queries for cooldowns can fail or be unavailable in test/dev contexts.
  Mitigation: fail soft to an empty cooldown list and keep the rest of the dashboard useful.
- Risk: auto-starting the endpoint changes boot behavior outside manual dashboard use.
  Mitigation: keep the change narrow, cover it with focused tests, and verify full conductor boot/test flow after the endpoint change.
