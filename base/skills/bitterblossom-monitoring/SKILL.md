---
name: bitterblossom-monitoring
user-invocable: true
description: "Monitor and recover Bitterblossom sprite tasks using status, watchdog, wait mode, and targeted diagnostics when dispatch appears stalled."
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
bb logs <sprite> --lines 50
```

## During Active Dispatch

Prefer live logs:

```bash
bb logs <sprite> --follow
```

If the sprite looks stuck or a prior dispatch was interrupted:

```bash
bb kill <sprite>
bb status <sprite>
bb logs <sprite> --lines 100
```

## Fast Triage Heuristics

- `bb logs --follow` is streaming: the task is active.
- `bb status <sprite>` shows `BLOCKED.md`: inspect the sprite workspace and unblock explicitly.
- `bb status <sprite>` shows recent commits or PRs: the task likely produced work even if the prior terminal session ended.
- `bb kill <sprite>` succeeds cleanly: the sprite is ready for a fresh dispatch.

## Direct Sprite Probe (Fallback)

Use only when BB surfaces are insufficient:

```bash
sprite exec -o "${SPRITES_ORG:-personal}" -s <sprite> -- bash -lc 'ls -la /home/sprite/workspace'
```

Add timeout for unstable exec calls:

```bash
timeout 20 sprite exec -o "${SPRITES_ORG:-personal}" -s <sprite> -- pwd
```
