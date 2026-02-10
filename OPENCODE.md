# OPENCODE.md — Project Context for OpenCode Agents

## What This Is

Bitterblossom is a Go CLI (`bb`) for managing coding agent sprites on Fly.io. It handles fleet lifecycle, dispatch, monitoring, and composition management.

## Quick Start

```bash
# Build
go build ./cmd/bb

# Test
go test ./...

# Lint
golangci-lint run ./...
```

## Architecture

```
cmd/bb/           → CLI commands (cobra)
internal/
  agent/          → Agent supervisor, heartbeat, progress tracking
  contracts/      → Shared interfaces and types
  dispatch/       → Task dispatch to sprites
  fleet/          → Fleet composition management
  lifecycle/      → Sprite lifecycle (create, destroy, settings)
  provider/       → Model provider configuration (OpenRouter, Claude)
base/             → Base config files pushed to sprites
compositions/     → YAML fleet composition definitions
docs/             → Design docs and references
```

## Key Patterns

- **Cobra CLI**: All commands in `cmd/bb/`. Use `cobra.Command` with `RunE`.
- **Table-driven tests**: See `internal/provider/provider_test.go` for examples.
- **Error handling**: Always check and wrap errors. Use `fmt.Errorf("%w: ...", err)`.
- **Provider abstraction**: `internal/provider/` maps model strings to env vars and CLI flags.

## Coding Standards

- Go 1.23+
- `gofmt` + `golangci-lint`
- Semantic commit messages: `feat:`, `fix:`, `test:`, `docs:`, `refactor:`
- Tests required for all new functionality
- No bash scripts — everything is Go

## Sprites

Sprites are standalone Linux VMs from [sprites.dev](https://sprites.dev). They are NOT Fly.io Machines.
- CLI: `sprite` (at `~/.local/bin/sprite`)
- Each sprite has 100GB persistent filesystem
- Sprites auto-sleep when idle
- Use `sprite exec -s NAME -- CMD` to run commands on sprites

## Current Migration

We are migrating from Claude Code (with OpenRouter hack) to OpenCode CLI with native OpenRouter support. The `opencode.json` config and `.opencode/agents/` directory define how OpenCode operates in this repo.
