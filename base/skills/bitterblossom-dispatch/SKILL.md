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
```

Confirm:
- `GITHUB_TOKEN` is set.
- Sprite is reachable from `bb status`.

## Workflow

1. Select a sprite that is reachable and not busy.
2. Fetch issue context locally and embed it in the prompt body.
3. Dispatch:

```bash
bb dispatch <sprite> "<prompt with embedded issue context>" --repo <owner/repo> --timeout 25m
```

4. Monitor while running:

```bash
bb logs <sprite> --follow --lines 100
```

5. If the run is stalled:

```bash
bb status <sprite>
bb kill <sprite>
```

## Failure Handling

- If dispatch exits non-zero, capture stderr and classify whether it is:
  - credential/env failure
  - reachability failure
  - active-loop/busy guard
  - off-rails or agent failure
