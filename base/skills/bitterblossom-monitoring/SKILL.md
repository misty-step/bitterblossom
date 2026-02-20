---
name: bitterblossom-monitoring
user-invocable: true
description: "Monitor and recover Bitterblossom sprite tasks using status, logs, kill, and targeted diagnostics when dispatch appears stalled."
allowed-tools:
  - Read
  - Grep
  - Glob
  - Bash
---

# Bitterblossom Monitoring

Use when a dispatched task might be stuck, blocked, or silent.

## Primary Checks

```bash
source .env.bb
bb status
bb status <sprite>
bb logs <sprite> --lines 100
```

## During Active Dispatch

Prefer:

```bash
bb logs <sprite> --follow --lines 100
```

If output is silent for too long:

```bash
bb status <sprite>
bb kill <sprite>
```

## Fast Triage Heuristics

- `active dispatch loop`: sprite is busy; avoid overlapping dispatch.
- `unreachable`: infrastructure/transport problem; retry later or pick another sprite.
- `TASK_COMPLETE present`: task completed.
- `BLOCKED.md present`: agent cannot proceed without intervention.

## Direct Sprite Probe (Fallback)

Use only when BB surfaces are insufficient:

```bash
sprite exec -o "$FLY_ORG" -s <sprite> -- bash -lc 'ls -la /home/sprite/workspace'
```

Add timeout for unstable exec calls:

```bash
timeout 20 sprite exec -o "$FLY_ORG" -s <sprite> -- pwd
```
