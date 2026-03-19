# Conductor

The Elixir conductor is Bitterblossom's control plane. It owns leasing, dispatch, governance, merge authority, and durable run state.

## Runtime Contract

[WORKFLOW.md](../WORKFLOW.md) is the primary workflow contract. The conductor executes that contract; it does not redefine it.

Local state lives in:

- `.bb/conductor.db`
- `.bb/events.jsonl`

## Commands

```bash
cd conductor
mix deps.get
mix compile
mix conductor check-env
mix conductor fleet --fleet ../fleet.toml --reconcile
mix conductor start --fleet ../fleet.toml
```

Inspection commands:

```bash
cd conductor
mix conductor fleet --fleet ../fleet.toml
mix conductor logs bb-weaver --follow
mix conductor show-runs --limit 10
mix conductor show-events --run_id <run-id>
mix conductor show-incidents --run_id <run-id>
mix conductor show-waivers --run_id <run-id>
```

## Fleet Reconciliation

`mix conductor fleet --reconcile` is the supported repair path. It:

1. probes sprite reachability
2. uploads base config and skills
3. installs Codex if needed
4. configures GitHub auth and git credential helper
5. ensures the repo mirror exists on the sprite
6. writes workspace metadata

There is no separate `bb setup` step anymore.

## Logs

`mix conductor logs <sprite>` tails `ralph.log` from the active or most recent workspace on a sprite. Dispatch now tees agent output into that log file so operators can inspect work in flight or after disconnects.

## Recovery

The conductor preflights dispatch by killing stale agent processes. For manual recovery from an IEx shell:

```elixir
Conductor.Sprite.kill("bb-weaver")
```

## Remote Deployment

Run the conductor on a persistent coordinator sprite, not a laptop shell that can sleep or drift.

Typical bootstrap:

```bash
sprite create coordinator
sprite exec coordinator -- bash -lc '
  cd /home/sprite/workspace/bitterblossom/conductor &&
  mix deps.get &&
  mix conductor fleet --fleet ../fleet.toml --reconcile
'
```
