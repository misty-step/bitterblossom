# Bitterblossom QA Runbook

This runbook covers the supported Bitterblossom surface:

- `mix conductor ...` is the operator CLI
- `Conductor.Fleet.Reconciler` provisions and repairs sprites
- `Conductor.Sprite` owns remote exec, logs, and recovery helpers

## Regression Checks

```bash
make test
cd conductor && mix test
python3 -m pytest -q base/hooks/ scripts/test_runtime_contract.py
```

## Required Environment

```bash
source .env.bb
export GITHUB_TOKEN="$(gh auth token)"
export SPRITE_TOKEN="..."         # preferred
# or export FLY_API_TOKEN="..."   # fallback
```

## Fleet Smoke Test

```bash
cd conductor
mix conductor fleet --fleet ../fleet.toml --reconcile
mix conductor fleet --fleet ../fleet.toml
mix conductor logs bb-weaver --lines 50
```

If a sprite is stuck, recover it from Elixir or a console:

```elixir
Conductor.Sprite.kill("bb-weaver")
```

## Conductor Smoke Test

```bash
cd conductor
mix conductor check-env
mix conductor start --fleet ../fleet.toml
```

In another shell:

```bash
cd conductor
mix conductor show-runs --limit 5
mix conductor show-events --run_id <run-id>
```

## Manual Checklist

- `mix conductor fleet --reconcile` provisions unhealthy sprites without `bb setup`.
- `mix conductor logs <sprite> [--follow]` tails the sprite log file.
- `mix conductor fleet` reports health truthfully after reconciliation.
- Elixir tests pass without any Go build or `cmd/bb/` dependency.
