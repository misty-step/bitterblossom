# Multi-repo agent support

Priority: high
Status: done
Estimate: M

## Goal
Each sprite can work across multiple repos. fleet.toml already supports `repo` per sprite defaults. Agents reference their assigned repo in the loop prompt. Infrastructure provisions workspace per repo.

## Non-Goals
- Cross-repo PRs (one sprite, one repo per loop iteration)

## Oracle
- [x] fleet.toml can declare sprites assigned to different repos
- [x] Launcher passes repo to loop prompt
- [x] Agent picks issues from its assigned repo
- [x] Tested with at least 2 repos

## What Was Built
- Sprite workspaces now use the full `owner/repo` path instead of only the repo basename, so the same sprite can safely switch between same-basename repos without aliasing the checkout.
- Loop dispatch now exports `REPO` into the workspace runtime env, which makes the Weaver, Thorn, and Fern AGENTS commands target the assigned repo instead of an empty shell variable.
- Launcher relaunch now self-heals old-layout or missing repo checkouts by reprovisioning the sprite before restarting the loop.
- Coverage now proves repo overrides through fleet loading, CLI start, application launch, health-monitor relaunch, launcher migration, runtime env upload, and same-basename repo declarations.

## Verification
- [x] `cd conductor && mix compile --warnings-as-errors`
- [x] `cd conductor && mix test test/conductor/application_test.exs test/conductor/cli_fleet_test.exs test/conductor/fleet/health_monitor_test.exs test/conductor/fleet/loader_test.exs test/conductor/launcher_test.exs test/conductor/sprite_agent_test.exs test/conductor/sprite_test.exs test/conductor/workspace_test.exs`

## Workarounds
- None. The workspace collision was fixed directly instead of being papered over with a loader guard.
