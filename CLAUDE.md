# CLAUDE.md

Claude-family tools may read this file first. Keep it aligned w `AGENTS.md`.

Also read:
- `WORKFLOW.md` (repo-owned runtime workflow contract)
- `AGENTS.md` (canonical repo context)
- `docs/adr/004-elixir-conductor-architecture.md` (Elixir conductor design)

## Current Direction Lock

Bitterblossom is currently focused on one lane: `Tansy` watches Canary,
investigates incidents, fixes the correct repository, and verifies recovery.
The older builder/fixer/polisher lanes remain as prior art and may return, but
they are not the active product priority right now.

## What This Is

Bitterblossom is an agent-first software factory. Autonomous AI agents (sprites) pick work, implement it, review it, and merge it. The codebase has two concerns:

1. **Infrastructure** (`conductor/`): Elixir/OTP — provisions sprites, bootstraps harnesses, dispatches agent loops, monitors health. Thin. No judgment.
2. **Agent definitions** (`sprites/`): AGENTS.md, CLAUDE.md, and skill files that define what each agent does. This is where the real logic lives.

Agents are capable. The infrastructure is plumbing.

## Sprite Names

- **Weaver** (`bb-builder`) — autonomous builder: picks `backlog.d/` items, shapes, implements via `/autopilot`, and prepares branches for local review
- **Thorn** (`bb-fixer`) — autonomous fixer: scans local finding ledgers and verification failures, resolves blockers, and restores land-readiness via `/settle`
- **Fern** (`bb-polisher`, `bb-polisher-2`, `bb-polisher-3`) — autonomous quality + merger: reviews, polishes, refactors, and squash-lands via `/settle`
- **Muse** (`bb-muse`) — reflects on runs and synthesizes learning for the factory
- **Tansy** (`bb-tansy`) — Canary incident responder: claims incidents, investigates root causes, repairs target repos, and verifies recovery

## Architecture

```text
conductor/                   Infrastructure only — no judgment
  lib/conductor/
    sprite.ex                Sprite provisioning, exec, health
    workspace.ex             Worktree lifecycle on sprites
    bootstrap.ex             Spellbook clone + bootstrap on sprites
    launcher.ex              Dispatch agent loops, monitor health
    store.ex                 SQLite event log for observability
    fleet/                   Fleet loading, reconciliation, health
    config.ex                Runtime configuration
    cli.ex                   bitterblossom start/stop/status

sprites/                     Agent definitions — where the logic lives
  shared/                    Shared CLAUDE.md, AGENTS.md, skills
  weaver/                    Autonomous builder loop
  thorn/                     Autonomous fixer loop
  fern/                      Autonomous polisher+merger loop
  tansy/                     Autonomous Canary incident responder loop

base/                        Skills/configs uploaded to all sprites
```

## Operating Model

1. `cd conductor && mix conductor start --fleet ../fleet.toml` provisions sprites, bootstraps spellbook, dispatches agent loops
2. Each sprite runs its own autonomous loop (defined in its AGENTS.md)
3. Weaver picks `backlog.d/` items → Thorn fixes local blockers → Fern polishes and lands
4. Self-healing: verification failures or conflicts → Thorn → Fern re-polishes → land

For the current direction lock, the default fleet runs only `bb-tansy`.

## Build & Test

```bash
cd conductor && mix deps.get && mix compile && mix test
```

## Agent-First Philosophy

**Agents are capable.** They can pick backlog items, classify failures, decide what to fix, evaluate quality, and merge. The infrastructure exists only to give them a healthy environment.

**No conductor judgment.** The conductor doesn't decide which issues to work, how to fix CI, whether to retry, or what merge policy to apply. Agents make all those decisions via their AGENTS.md definitions and skills.

**Skills are the unit of capability.** `/autopilot`, `/settle`, `/code-review`, `/debug`, `/shape`, `/reflect` — these are the building blocks. Agents compose them based on what they observe.

**Spellbook is the canonical skill set.** `phrazzld/spellbook` defines the skills and agents. Sprites are bootstrapped with it before every dispatch.

**Self-healing cycles.** Weaver prepares a branch → Thorn fixes blockers → Fern polishes and lands locally. `backlog.d/` is the canonical work source. No dead ends, no stuck states.

## Gotchas (earned by pain)

- **Stale agent processes block dispatch.** Sprite agent processes may linger after a run completes. Kill before re-dispatch.
- **Issue boundaries must not contradict AC.** Ensure acceptance criteria are achievable within stated constraints.

## Coding Standards

- Elixir 1.16+, `mix format`, deep modules (Ousterhout)
- Semantic commits: `feat:`, `fix:`, `test:`, `docs:`, `refactor:`
- Code is a liability — every line fights for its life
