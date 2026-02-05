# OpenClaw Integration

How Kaylee (OpenClaw) uses Bitterblossom to coordinate the sprite fleet.

## Routing

Kaylee reads `agents.yaml` to match incoming tasks to sprites. The routing algorithm:

1. Extract keywords from task description
2. Score each sprite by keyword overlap with their keyword list
3. Apply rule overrides (high-priority patterns like "bug" → Thorn)
4. Route to highest-scoring sprite
5. If tied or no match, use fallback (Bramble)

## Task Lifecycle

```
1. Task arrives (GitHub issue, PR, manual request)
2. Kaylee analyzes task and extracts routing signals
3. Kaylee selects sprite based on agents.yaml
4. Sprite receives task, implements, pushes to branch
5. GitHub Action runs multi-model PR review
6. Kaylee logs observation in observations/OBSERVATIONS.md
7. Iterate based on patterns in observations
```

## Composition Changes

When Kaylee observes patterns that suggest composition changes:

1. Document observation in `observations/OBSERVATIONS.md`
2. Propose change to `compositions/v1.yaml` (or create v2.yaml)
3. Human reviews and approves composition change
4. Run `scripts/sync.sh` to push updates, or `scripts/provision.sh` for new sprites
5. Continue observing

## Constraints

- Kaylee coordinates but doesn't implement
- Sprites are full-stack; routing is preference, not restriction
- PR review is handled by a separate GitHub Action, not by sprites
- All sprites share a GitHub account
- Human approval required for composition changes
