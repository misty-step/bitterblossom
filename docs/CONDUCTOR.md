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

Before reconciling or starting the fleet, log into Codex locally with file-backed credentials so the conductor can seed `~/.codex/auth.json` onto managed sprites:

```bash
mkdir -p "${CODEX_HOME:-$HOME/.codex}"
grep -q 'cli_auth_credentials_store = "file"' "${CODEX_HOME:-$HOME/.codex}/config.toml" 2>/dev/null || \
  printf '\ncli_auth_credentials_store = "file"\n' >> "${CODEX_HOME:-$HOME/.codex}/config.toml"
codex login
```

If local Codex auth is unavailable, the conductor falls back to `OPENAI_API_KEY` for Codex dispatches.

Inspection commands:

```bash
cd conductor
mix conductor status
mix conductor fleet --fleet ../fleet.toml
mix conductor logs bb-weaver --follow
mix conductor show-events --limit 50
```

## Fleet Reconciliation

`mix conductor fleet --reconcile` is the supported repair path. It:

1. probes sprite reachability
2. uploads base config and skills
3. installs Codex if needed
4. seeds `~/.codex/auth.json` when local Codex account auth is available
5. configures GitHub auth and git credential helper
6. ensures the repo mirror exists on the sprite
7. writes workspace metadata

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
