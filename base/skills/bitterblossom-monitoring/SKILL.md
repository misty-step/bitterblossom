---
name: bitterblossom-monitoring
user-invocable: true
description: "Inspect Bitterblossom fleet health and sprite task logs through the Elixir conductor."
allowed-tools:
  - Read
  - Grep
  - Glob
  - Bash
---

# Bitterblossom Monitoring

Use when a conductor-managed sprite might be stuck, blocked, or silent.

## Primary Checks

```bash
cd conductor
mix conductor fleet --fleet ../fleet.toml
mix conductor logs <sprite> --lines 50
```

## During Active Work

Prefer live logs:

```bash
cd conductor
mix conductor logs <sprite> --follow
```

If a sprite looks unhealthy, reconcile first:

```bash
cd conductor
mix conductor fleet --fleet ../fleet.toml --reconcile
```

## Fast Triage Heuristics

- `mix conductor logs --follow` is streaming: the task is active.
- `mix conductor fleet` shows `needs setup`: reconcile the fleet.
- `mix conductor fleet` shows `unreachable`: use direct `sprite` diagnostics.

## Direct Sprite Probe (Fallback)

Use only when conductor surfaces are insufficient:

```bash
sprite exec -o "${SPRITES_ORG:-personal}" -s <sprite> -- bash -lc 'ls -la /home/sprite/workspace'
```
