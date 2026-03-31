# Kill the orchestration layer

Priority: critical
Status: ready
Estimate: L

## Goal
Delete the run-centric orchestration code from the conductor, reducing it from ~6K LOC to ~2.5K LOC. The conductor becomes a thin CRUD wrapper: provision sprites, observe health, serve dashboard. All judgment, governance, and workspace management belong to the sprites.

## Non-Goals
- Rewrite the conductor from scratch (delete and reshape, don't rebuild)
- Move governance logic into a new location (sprites already have `/settle`, `/code-review` — they own it)
- Preserve backward compatibility with the old dispatch path

## Sequence

### Phase 1: Delete dead orchestration modules
- [ ] Delete `Conductor.GitHub` entirely (868 LOC) — zero inbound calls from surviving modules
- [ ] Gut `Conductor.Store` — drop `runs`, `leases`, `incidents`, `waivers` tables and all their CRUD functions (~600 LOC); keep `events` table and `control` table only (~150 LOC surviving)
- [ ] Delete `Conductor.Workspace` worktree lifecycle — `prepare/5`, `rebase/3`, `adopt_branch/5`, `cleanup/4`, `health_check/4` (~400 LOC). Keep `sync_persona` and `repo_root` (~150 LOC), extract to `Conductor.Persona`
- [ ] Delete orchestration config knobs from `Conductor.Config` — `builder_timeout`, `builder_retry_*`, `ci_timeout`, `max_concurrent_runs`, `max_replays`, `stale_run_threshold_minutes`, etc. (~200 LOC)

### Phase 2: Reshape surviving modules
- [ ] `Conductor.Sprite` (1,147 → ~500 LOC) — remove `dispatch/4` (old sync orchestration), `run_agent`, `active_worktree`. Keep exec/provision/status/logs/start_loop/stop_loop/pause/resume
- [ ] `Conductor.CLI` (561 → ~250 LOC) — remove `launch_with_restart` auto-restart loop, workspace references. Keep: `start` (reconcile + health monitor + dashboard), `fleet status`, `sprite {start,stop,pause,resume,status,logs}`, `dashboard`, `check-env`
- [ ] `Conductor.Application` (147 → ~60 LOC) — remove dispatch loops. Boot becomes: load fleet.toml → reconcile sprites → start health monitor → start dashboard
- [ ] `Conductor.Launcher` (142 → ~80 LOC) — remove `reset_workspace`, `rescue_unpushed_script`, sync dispatch path. Keep: provision check, bootstrap spellbook, sync persona, build prompt, `Sprite.start_loop`
- [ ] `Conductor.Harness` (74 → ~40 LOC) — delete `classify_dispatch_failure` and `retry_backoff_ms`, keep behaviour definition
- [ ] `Conductor.Web.DashboardLive` — transform from run-centric to sprite-centric: sprite health grid + event stream (no more runs table)

### Phase 3: Clean up
- [ ] Delete all tests for removed modules/functions
- [ ] `mix compile --warnings-as-errors` clean
- [ ] `mix test` green
- [ ] `mix format` clean

## Oracle
- [ ] `Conductor.GitHub` module does not exist
- [ ] `Store` has exactly two tables: `events` and `control` — no `runs`, `leases`, `incidents`, `waivers`
- [ ] `Conductor.Workspace` has no worktree functions — only persona sync (or extracted to `Conductor.Persona`)
- [ ] `mix xref graph --format stats` shows no references to deleted modules
- [ ] `mix test` passes
- [ ] `mix compile --warnings-as-errors` passes
- [ ] Total conductor LOC (excluding tests) is under 3,000
- [ ] Dashboard renders sprite health grid and event stream, not runs table

## Notes
This is the architectural pivot. Every other backlog item assumes this is done first.

Simplifier's module-by-module verdict provides the detailed cut list. Mapper confirmed GitHub.ex has zero inbound calls. Store run/lease machinery is only called by code that itself gets deleted.

Target module list after completion:
```
conductor/lib/conductor/
  application.ex      ~60 LOC
  cli.ex              ~250 LOC
  config.ex           ~150 LOC
  shell.ex            79 LOC (unchanged)
  sprite.ex           ~500 LOC
  sprite_cli_auth.ex  99 LOC (unchanged)
  bootstrap.ex        60 LOC (unchanged)
  launcher.ex         ~80 LOC
  persona.ex          ~150 LOC (extracted from workspace)
  harness.ex          ~40 LOC
  codex.ex            44 LOC (unchanged)
  claude_code.ex      48 LOC (unchanged)
  store.ex            ~150 LOC
  fleet/
    loader.ex         203 LOC (unchanged)
    reconciler.ex     268 LOC (unchanged)
    health_monitor.ex ~120 LOC
  web/
    endpoint.ex       31 LOC (unchanged)
    router.ex         18 LOC (unchanged)
    layouts.ex        50 LOC (unchanged)
    dashboard_live.ex ~180 LOC
```
