# CLAUDE.md

Claude-family tools may read this file first. Keep it aligned w `AGENTS.md`.

Also read:
- `WORKFLOW.md` (repo-owned runtime workflow contract)
- `AGENTS.md` (canonical repo context)
- `docs/adr/004-elixir-conductor-architecture.md` (Elixir conductor design)

## What This Is

Bitterblossom has two surfaces:

- `conductor/`: Elixir/OTP orchestrator — leases issues, dispatches builders, governs PRs, merges
- `cmd/bb/`: Go transport for sprite dispatch, setup, status, logs (being absorbed into Elixir per #621)

The conductor owns workflow judgment and durable run state. `bb` is transitional transport.

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
scripts/ralph.sh         Agent loop on sprites (being eliminated, see #621)
```

## Operating Model

1. `cd conductor && mix conductor run-once --repo R --issue N --worker W` runs one issue
2. `cd conductor && mix conductor loop --repo R --worker W1 --worker W2` runs continuously
3. `cd conductor && mix conductor show-runs` / `show-events` for inspection

## Build & Test

```bash
# Elixir conductor
cd conductor && mix deps.get && mix compile && mix test

# Go transport (transitional)
go build -o bin/bb ./cmd/bb
```

## Coding Standards

- Elixir 1.16+, `mix format`, deep modules (Ousterhout)
- Go 1.23+, `gofmt` + `golangci-lint`
- Semantic commits: `feat:`, `fix:`, `test:`, `docs:`, `refactor:`
- No new Go packages. Go surface is shrinking, not growing.
