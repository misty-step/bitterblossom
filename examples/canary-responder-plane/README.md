# Canary responder plane

This is a credential-free starter plane for the Tansy/Canary incident responder
workload. It carries forward the safe part of the April Tansy intent in the
current v3 shape: task, agent, trigger, budget, and lane card files, with no
Elixir conductor surface and no workload-specific Rust.

Validate it with:

```sh
bb --config examples/canary-responder-plane check
```

Runtime dispatch needs:

- `OPENROUTER_API_KEY`
- `CANARY_ENDPOINT`
- `CANARY_API_KEY`
- `GH_TOKEN`
- `BB_HOOK_CANARY` when using webhook ingress

Manual run shape:

```sh
bb --config examples/canary-responder-plane run canary-incident \
  --payload '{"incident_id":"inc_123","service":"canary","dry_run":true}'
```

Webhook events are wake-up hints only. The responder card requires the agent to
re-read Canary incidents, reports, and timelines before selecting a repo or
suggesting a fix.

The example webhook filter admits only `incident.opened` and
`incident.updated` for the `canary` service. Broaden that allowlist in
`tasks/canary-incident/task.toml` before wiring a wider Canary subscription.
