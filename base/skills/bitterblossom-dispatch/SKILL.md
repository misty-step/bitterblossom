---
name: bitterblossom-dispatch
user-invocable: true
description: "Dispatch a GitHub issue or prompt to a Bitterblossom sprite with explicit skill mounting, safe dry-run planning, and wait-mode monitoring."
allowed-tools:
  - Read
  - Grep
  - Glob
  - Bash
---

# Bitterblossom Dispatch

Run this skill when you want a sprite to execute a coding task through `bb dispatch`.

## Preflight

```bash
source .env.bb
bb status
bb dispatch <sprite> "dry-run readiness probe" --repo <owner/repo> --dry-run
```

Confirm:
- `GITHUB_TOKEN` is set.
- `SPRITE_TOKEN` is preferred, or `FLY_API_TOKEN` is available as fallback auth.
- Target sprite is already set up for the repo (`bb setup <sprite> --repo <owner/repo>`).

## Workflow

1. Probe readiness first:

```bash
bb dispatch <sprite> "dry-run readiness probe" --repo <owner/repo> --dry-run
```

2. Dispatch the real task:

```bash
bb dispatch <sprite> "Implement feature X" --repo <owner/repo>
```

3. Follow progress and verify output:

```bash
bb logs <sprite> --follow
bb status <sprite>
```

## Failure Handling

- If readiness fails, re-run setup:

```bash
bb setup <sprite> --repo <owner/repo> --force
```

- If dispatch was interrupted or the sprite is stuck, recover with:

```bash
bb kill <sprite>
bb logs <sprite> --lines 50
bb status <sprite>
```
