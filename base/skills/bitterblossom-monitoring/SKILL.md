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
bb status --format text
bb status <sprite> --format text
bb watchdog --sprite <sprite>
```

## During Active Dispatch

Prefer:

```bash
bb dispatch <sprite> ... --execute --wait
```

If wait mode is silent for too long:

```bash
bb status <sprite> --format text
bb watchdog --sprite <sprite> --json
```

## Fast Triage Heuristics

- `running`: task active; continue waiting.
- `blocked`: inspect `/home/sprite/workspace/BLOCKED.md`.
- `complete`: pull PR URL or branch changes from sprite workspace.
- `dead` or `stale`: re-dispatch with same prompt and capture logs.

## Direct Sprite Probe (Fallback)

Use only when BB surfaces are insufficient:

```bash
sprite exec -o "$FLY_ORG" -s <sprite> -- bash -lc 'ls -la /home/sprite/workspace'
```

Add timeout for unstable exec calls:

```bash
timeout 20 sprite exec -o "$FLY_ORG" -s <sprite> -- pwd
```

