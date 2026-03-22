# CLAUDE.md

Claude-family tools may read this file first. Keep it aligned w `AGENTS.md`.

Also read:
- `WORKFLOW.md` (repo-owned runtime workflow contract)
- `AGENTS.md` (canonical repo context)
- `docs/adr/004-elixir-conductor-architecture.md` (Elixir conductor design)

## What This Is

Bitterblossom has two surfaces:

- `conductor/`: Elixir/OTP orchestrator — leases issues, dispatches named sprites, governs PRs, merges
- `cmd/bb/`: Go transport for sprite dispatch, setup, status, logs (being absorbed into Elixir per #621)

The conductor owns workflow judgment and durable run state. `bb` is transitional transport.

## Sprite Names

- **Weaver** (`bb-weaver`) — implements issues, writes code, opens PRs
- **Thorn** (`bb-thorn`) — watches failing CI and pushes narrow fixes
- **Fern** (`bb-fern`) — reviews PRs, polishes diffs, adds `lgtm` when warranted
- **Muse** (`bb-muse`) — reflects on runs and synthesizes learning for the factory

## Architecture

```text
conductor/
  lib/conductor/
    application.ex       OTP supervision tree
    orchestrator.ex      Polling loop, issue selection, run dispatch
    run_server.ex        Per-run GenServer — state machine lifecycle
    store.ex             SQLite persistence (runs, leases, events)
    github.ex            GitHub operations via gh CLI
    sprite.ex            Sprite operations via sprite/bb CLI
    workspace.ex         Worktree lifecycle on sprites
    prompt.ex            Builder prompt construction
    shell.ex             Subprocess execution with timeout
    config.ex            Runtime configuration
    issue.ex             Issue struct + readiness checks
    cli.ex               CLI commands

cmd/bb/                  Go transport (transitional, see #621)
scripts/                 Prompt templates, onboarding helpers, runtime contract tests
```

## Operating Model

1. `cd conductor && mix conductor start --fleet ../fleet.toml` boots the full pipeline
2. `cd conductor && mix conductor pause` / `resume` controls dispatch without stopping workers
3. `cd conductor && mix conductor show-runs` / `show-events` for inspection

## Build & Test

```bash
# Elixir conductor
cd conductor && mix deps.get && mix compile && mix test

# Go transport (transitional)
go build -o bin/bb ./cmd/bb
```

## Conductor Authorities

The conductor owns five authorities. Nothing else may perform these:

1. **Lease** — claim an issue so no other run touches it
2. **Dispatch** — send Weaver to a sprite with a prompt and a worktree
3. **Govern** — independently verify: CI green? Reviews clean? Policy satisfied?
4. **Merge** — squash-merge when governance passes
5. **Learn** — post-run retro analysis, backlog synthesis, pattern detection

The entity doing the work cannot judge the work. Weaver doesn't know the merge policy.

## Gotchas (earned by pain, 2026-03-14)

- **Go/Elixir coupling on file paths.** The Go transport (`cmd/bb/`) and Elixir conductor still share runtime path assumptions such as workspace roots, prompt-template paths, and signal filenames. Always grep both `cmd/bb/` and `conductor/` when changing those contracts.
- **Closing a PR doesn't stop the conductor.** The conductor doesn't monitor PR state — it only checks CI status. To stop a merge, use the `hold` label on the issue (#637). Closing the PR is not communication.
- **`statusCheckRollup` contains null entries.** External review tools (CodeRabbit) report checks with null conclusions. `evaluate_checks/1` filters these. If you add new CI check logic, handle nulls.
- **Stale agent processes block dispatch.** When runs complete, sprite agent processes may linger on the sprite. The next dispatch detects them and refuses. The conductor retries every 60s until they die.
- **Issue boundaries must not contradict AC.** If you write "Don't modify X" but AC requires X to compile, Weaver will modify X. Ensure AC is achievable within stated boundaries.

## Coding Standards

- Elixir 1.16+, `mix format`, deep modules (Ousterhout)
- Go 1.23+, `gofmt` + `golangci-lint`
- Semantic commits: `feat:`, `fix:`, `test:`, `docs:`, `refactor:`
- No new Go packages. Go surface is shrinking, not growing.
