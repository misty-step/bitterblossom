# AGENTS.md

Universal project context for all coding agents (OpenCode, Codex, Claude Code, etc.).

## What This Is

Bitterblossom is a Go CLI (`bb`) for managing coding agent sprites on Fly.io. It handles fleet lifecycle, dispatch, monitoring, and composition management. This is infrastructure-as-config for AI agent orchestration.

## Key Concepts

- **Sprites** = Isolated Linux sandboxes with persistent 100GB filesystems, auto-sleep. Standalone service at [sprites.dev](https://sprites.dev) — NOT Fly.io Machines. Use the `sprite` CLI, not `fly machines`.
- **Compositions** = Team hypotheses. YAML files defining which sprites exist and their preferences.
- **Base config** = Shared engineering philosophy, hooks, skills inherited by all sprites.
- **Persona** = Individual sprite identity and specialization (in `sprites/`).
- **Observations** = Learning journal tracking what works and what doesn't.

## Architecture

```
cmd/bb/            → Go CLI control plane (Cobra)
internal/          → Core packages
  agent/           → Agent supervisor, heartbeat, progress tracking
  contracts/       → Shared interfaces and types
  dispatch/        → Task dispatch to sprites
  fleet/           → Fleet composition management
  lifecycle/       → Sprite lifecycle (create, destroy, settings)
  provider/        → Model provider configuration (OpenRouter, Claude)
  monitor/         → Fleet monitoring
  watchdog/        → Health checks + auto-recovery
pkg/               → Shared libraries (fly, events)
base/              → Shared config pushed to every sprite
compositions/      → Team hypotheses (YAML)
scripts/           → Legacy shell scripts (being replaced by Go)
docs/              → CLI reference, contracts, design docs
```

## CLI

Primary interface is `bb` (Go CLI). See `docs/CLI-REFERENCE.md` for full reference.

| Command | Purpose |
|---------|---------|
| `bb provision` | Create sprite + upload config |
| `bb sync` | Push config updates to running sprites |
| `bb teardown` | Export data + destroy sprite |
| `bb dispatch` | Send a task to a sprite |
| `bb status` | Fleet overview or sprite detail |
| `bb watchdog` | Fleet health checks + auto-recovery |
| `bb compose` | Composition-driven reconciliation |
| `bb agent` | On-sprite supervisor (start, stop, status, logs) |
| `bb watch` | Real-time event stream dashboard |
| `bb logs` | Historical event log queries |

Build: `go build -o bb ./cmd/bb`

## Agent Kinds

The supervisor supports multiple coding agent backends:

| Kind | CLI | Notes |
|------|-----|-------|
| `codex` | `codex --yolo` | OpenAI Codex CLI |
| `opencode` | `opencode run -m MODEL --agent coder` | OpenCode with OpenRouter |
| `claude` | `claude -p` | Claude Code (legacy) |
| `kimi-code` | `kimi-code` | Kimi Code CLI (legacy) |

Default model for OpenCode: `openrouter/moonshotai/kimi-k2.5`

## Coding Standards

- **Go 1.23+**
- `gofmt` + `golangci-lint`
- Semantic commit messages: `feat:`, `fix:`, `test:`, `docs:`, `refactor:`
- Table-driven tests (see `internal/provider/provider_test.go` for examples)
- Handle errors explicitly — no `_` for error returns
- **No bash scripts** — write Go code for automation

## Hooks

Three hooks in `base/hooks/`:

| Hook | Trigger | Purpose |
|------|---------|---------|
| `destructive-command-guard.py` | PreToolUse (Bash) | Blocks destructive git ops |
| `fast-feedback.py` | PostToolUse (Edit/Write) | Auto-runs type checker after file edits |
| `memory-reminder.py` | Stop | Prompts sprite to update MEMORY.md |

## Important Rules

- **Sprites, not Machines.** Use `sprite` CLI, not `fly machines` or `flyctl`.
- **Tests required** for all new functionality.
- **Compositions are hypotheses.** Designed to be cheap to change and test.
- **Write code within 5 minutes.** Don't spend time analyzing endlessly — read the task, implement, test, commit.
- **Commit early, commit often.** Small atomic commits with semantic messages.
