# Watchpoints

Use this checklist while a supervised run is live.

## Preflight

- `mix compile --warnings-as-errors` clean in conductor/
- Environment resolves: GITHUB_TOKEN, SPRITE_TOKEN, Codex auth
- fleet.toml is valid and declares expected sprites
- At least one sprite is reachable and healthy

## Dispatch phase

- `mix conductor start` loads fleet and reconciles sprites
- Healthy sprites are seeded as `:launching` (not `:healthy`)
- Launcher preflight cleans stale processes before starting loops
- Loop starts are logged and health monitor tracks the transition
- Unhealthy sprites are deferred to HealthMonitor recovery

## Health monitoring

- HealthMonitor probes at configured interval
- `:launching` sprites transition to `:healthy` when loop confirmed alive
- `:launching` sprites time out after max ticks and degrade to `:unhealthy`
- `:healthy` sprites that lose their loop are detected and marked `:unhealthy`
- Recovery (`:unhealthy` → ready) triggers relaunch with preflight cleanup
- Store events recorded for all transitions (sprite_loop_confirmed, sprite_degraded, sprite_loop_exited, sprite_recovered)

## Sprite autonomous loop

- Weaver: picks an issue, shapes it, builds via /autopilot, opens PR
- Thorn: scans PRs for merge blockers, resolves conflicts, fixes CI via /settle
- Fern: reviews, polishes, refactors, squash-merges via /settle
- Each sprite reads its AGENTS.md and runs its own work loop
- Sprites re-observe repo state on each iteration (stateless across sessions)

## PR governance

- PR transitions from draft to ready at the intended point
- Required CI reruns after ready-for-review
- External review surfaces are observed, not ignored
- Unresolved threads, pending statuses, and merge policy are explicit

## Merge phase

- Merge occurs only after policy is truly settled
- GitHub timestamps align with store event timestamps
- No pending trusted review surface remains at merge time

## Post-run

- Backlog item, PR, and store events agree on terminal state
- Health monitor state is consistent (no sprites stuck in wrong state)
- Local operator path to understand what happened is short
- Friction converts into backlog updates, not hand-wavy notes
