# OpenClaw Integration

How Kaylee (OpenClaw) uses Bitterblossom to manage the sprite fleet.

## Routing

Kaylee reads `agents.yaml` for context about each sprite's strengths, then routes tasks using her own judgment. There is no programmatic routing algorithm — Kaylee is the intelligent coordinator.

Considerations when routing:
- What domain expertise does this task need most?
- Is a sprite already working in the same repo? (continuity > perfect match)
- What's the sprite's current workload? (don't overload one sprite)
- Is this an experiment? (try routing to a non-obvious sprite and observe)

## Task Lifecycle

```
1. Task arrives (GitHub issue, PR, manual request, cron)
2. Kaylee decides which sprite to route to
3. Kaylee dispatches: ./scripts/dispatch.sh <sprite> <prompt>
4. Sprite implements, commits, pushes to branch
5. GitHub Action runs multi-model PR review (separate system)
6. Kaylee logs observation in observations/OBSERVATIONS.md
7. Iterate based on patterns
```

## Dispatching Work

```bash
# Simple task
./scripts/dispatch.sh bramble "Build the REST API for user profiles"

# Task in a specific repo
./scripts/dispatch.sh thorn --repo misty-step/heartbeat "Add integration tests for the alerting system"

# Complex prompt from file
./scripts/dispatch.sh moss --file prompts/refactor-auth.md

# Long-running autonomous loop
./scripts/dispatch.sh fern --ralph "Investigate flaky CI failures and ship a fix"
```

## Observing Results

After every meaningful task, log an observation:

```markdown
### 2026-02-05 — Bramble — API Implementation
**Task:** Build user profile REST API
**Outcome:** Success
**Time:** ~25 min
**Notes:** Clean implementation, good error handling. Missed rate limiting.
**Action:** Keep routing API work to Bramble. Add rate limiting to standard prompts.
```

## Composition Changes

When observations suggest changes:

1. Document the pattern in `observations/OBSERVATIONS.md`
2. Create a new composition in `compositions/` (e.g., `v2.yaml`)
3. Get human approval
4. Provision new composition, decommission old
5. Continue observing and comparing

## Experimentation Ideas

- Route the same task to two different sprites, compare output
- Try a generalist composition (fewer sprites, no specialization)
- Try deeper specialization (more sprites, narrower focus)
- Change the base engineering philosophy, measure quality delta
- Run a sprite with different model configs and compare

## Fleet Management

```bash
# Check fleet status
./scripts/status.sh

# Sync config to all sprites after editing base/
./scripts/sync.sh

# Sync to just one sprite
./scripts/sync.sh bramble

# Provision a new sprite
./scripts/provision.sh <name>

# Decommission (exports MEMORY.md first)
./scripts/teardown.sh <name>
```
