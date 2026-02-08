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
3. Kaylee dispatches: bb dispatch <sprite> <prompt> --execute
4. Sprite implements, commits, pushes to branch
5. GitHub Action runs multi-model PR review (separate system)
6. Kaylee logs observation in observations/OBSERVATIONS.md
7. Iterate based on patterns
```

## Dispatching Work

```bash
# Simple task
bb dispatch bramble "Build the REST API for user profiles" --execute

# Task in a specific repo
bb dispatch thorn --repo misty-step/heartbeat "Add integration tests for the alerting system" --execute

# Complex prompt from file
bb dispatch moss --file prompts/refactor-auth.md --execute

# Long-running autonomous loop
bb dispatch fern --ralph "Investigate flaky CI failures and ship a fix" --execute

# JSON output for programmatic consumption
bb dispatch bramble "Fix the auth bug" --execute --json | jq '.data.state'
```

All dispatch commands are dry-run by default. Omit `--execute` to preview the dispatch plan without side effects.

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
# Fleet status
bb status --format text

# Composition vs actual state
bb compose status

# Fleet health check (dead/stale/blocked detection)
bb watchdog
bb watchdog --execute    # auto-redispatch dead sprites

# Sync config to all sprites after editing base/
bb sync

# Sync to just one sprite
bb sync bramble

# Provision a new sprite
bb provision <name>

# Decommission (exports MEMORY.md first)
bb teardown <name>

# Reconcile entire fleet to match composition
bb compose apply --execute
```

See [docs/CLI-REFERENCE.md](../docs/CLI-REFERENCE.md) for the full command reference.
