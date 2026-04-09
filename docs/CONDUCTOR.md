# Conductor

The Elixir conductor is Bitterblossom's infrastructure layer. It provisions
sprites, reconciles their runtime state, and exposes the operator commands that
support a local-first factory flow.

## Runtime Contract

[WORKFLOW.md](../WORKFLOW.md) is the primary workflow contract. The conductor
executes that contract; it does not redefine it.

Local state lives in:

- `.bb/conductor.db`
- `.bb/events.jsonl`

## Commands

```bash
make ci-fast

cd conductor
mix deps.get
mix compile
mix conductor fleet --fleet ../fleet.toml --reconcile
mix conductor fleet audit --fleet ../fleet.toml
mix conductor sprite status bb-tansy --fleet ../fleet.toml
mix conductor start --fleet ../fleet.toml
```

Before reconciling or starting the fleet, log into Codex locally with
file-backed credentials so the conductor can seed `~/.codex/auth.json` onto
managed sprites:

```bash
mkdir -p "${CODEX_HOME:-$HOME/.codex}"
grep -q 'cli_auth_credentials_store = "file"' "${CODEX_HOME:-$HOME/.codex}/config.toml" 2>/dev/null || \
  printf '\ncli_auth_credentials_store = "file"\n' >> "${CODEX_HOME:-$HOME/.codex}/config.toml"
codex login
```

If local Codex auth is unavailable, the conductor falls back to
`OPENAI_API_KEY` for Codex dispatches.

Inspection commands:

```bash
cd conductor
mix conductor status
mix conductor fleet --fleet ../fleet.toml
mix conductor fleet audit --fleet ../fleet.toml
mix conductor sprite start bb-tansy --fleet ../fleet.toml
mix conductor sprite pause bb-tansy --fleet ../fleet.toml --wait
mix conductor sprite resume bb-tansy --fleet ../fleet.toml
mix conductor sprite stop bb-tansy --fleet ../fleet.toml
mix conductor sprite status bb-tansy --fleet ../fleet.toml --json
mix conductor logs bb-tansy --follow
mix conductor show-events --limit 50
```

`mix conductor status` resolves directly from the fleet file and sprite probes.
It does not depend on a previously started local conductor process.

## Fleet Reconciliation

`mix conductor fleet --reconcile` is the supported repair path. It:

1. probes sprite reachability
2. uploads base config and skills
3. installs Codex if needed
4. seeds `~/.codex/auth.json` when local Codex account auth is available
5. configures repo transport auth when a sprite needs clone or publish access
6. ensures the repo mirror exists on the sprite
7. writes workspace metadata

There is no separate `bb setup` step anymore.

## Logs

`mix conductor logs <sprite>` tails `ralph.log` from the active or most recent
workspace on a sprite. Dispatch now tees agent output into that log file so
operators can inspect work in flight or after disconnects.

`mix conductor sprite logs <sprite>` is an alias for the same log surface.

## Local Landing

The conductor does not own semantic review or landing decisions. Those stay in
repo-native surfaces:

- `make ci-fast`
- `make ci`
- `scripts/land.sh <branch>`
- `refs/verdicts/<branch>`
- `.evidence/<branch>/<date>/`

Hosted remotes remain optional publication targets, not the workflow authority.

## Recovery

The conductor preflights dispatch by killing stale agent processes. For manual
recovery from an IEx shell:

```elixir
Conductor.Sprite.kill("bb-tansy")
```

## Direction

The direction is local-first, not remote-dashboard-first. The current conductor
lets declared sprites start, pause, resume, stop, inspect, and respond while
agents keep judgment in repo-owned skills and persona docs.

This slice is intentionally additive:

- the tracked root `fleet.toml` currently declares the minimal Tansy responder fleet
- alternate fleet files can still declare additional concrete `[[sprite]]` instances
- detached lifecycle control exists for declared sprites only
- template catalog, clone or create flows, and imperative scaling remain
  follow-up work tracked in
  [the context packet](./plans/2026-03-28-agent-first-fleet-cli.md)
