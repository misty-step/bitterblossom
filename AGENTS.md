# AGENTS.md

Universal project context for all coding agents working on Bitterblossom.

## What This Is

Bitterblossom is a Go CLI (`bb`) for managing coding agent sprites on Fly.io. It handles fleet lifecycle, dispatch, monitoring, and composition management. This is infrastructure-as-config for AI agent orchestration.

## Key Decision: Claude Code Is the Canonical Harness (Feb 10, 2026)

**Claude Code is the canonical sprite harness.** OpenCode is deprecated for sprite dispatch.

The Feb 9 OpenCode-only decision has been reversed. The proxy provider (PR #136) enables Claude Code to route through OpenRouter to any model (Kimi K2.5, GLM 4.7, etc.), eliminating the original limitation. Claude Code has superior tool use, proven PTY dispatch, and better ecosystem support.

See `docs/adr/001-claude-code-canonical-harness.md` for the full decision record.

## Key Concepts

- **Sprites** = Persistent Linux development environments with 100GB durable storage, auto-sleep, checkpoint/restore. They are NOT ephemeral containers. Create once, reuse forever.
- **Compositions** = Team hypotheses. YAML files defining which sprites exist and their preferences.
- **Base config** = Shared engineering philosophy, hooks, skills inherited by all sprites.
- **Persona** = Individual sprite identity and specialization (in `sprites/`).
- **Observations** = Learning journal tracking what works and what doesn't.
- **Checkpoint** = Instant (~300ms) snapshot of sprite state. Use after bootstrap and successful tasks.

## Architecture

```
cmd/bb/            → Go CLI control plane (Cobra)
internal/          → Core packages
  agent/           → Agent supervisor, heartbeat, progress tracking
  contracts/       → Shared interfaces and types
  dispatch/        → Task dispatch to sprites
  fleet/           → Fleet composition management
  lifecycle/       → Sprite lifecycle (create, provision, checkpoint, restore)
  provider/        → Model provider configuration (OpenRouter)
  monitor/         → Fleet monitoring
  watchdog/        → Health checks + auto-recovery
pkg/               → Shared libraries (fly, events)
base/              → Shared config pushed to every sprite
compositions/      → Team hypotheses (YAML)
docs/              → CLI reference, architecture, design docs
```

## CLI

Primary interface is `bb` (Go CLI). See `docs/CLI-REFERENCE.md` for full reference.

| Command | Purpose |
|---------|---------|
| `bb provision` | Create sprite + bootstrap env + checkpoint |
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

## Agent Configuration

### Canonical Harness: Claude Code (with Proxy Provider)

```bash
claude --yolo "TASK"
```

For non-Anthropic models via OpenRouter proxy:
```bash
ANTHROPIC_BASE_URL=https://openrouter.ai/api \
ANTHROPIC_AUTH_TOKEN="$OPENROUTER_API_KEY" \
ANTHROPIC_MODEL=moonshotai/kimi-k2.5 \
claude --yolo "TASK"
```

### Supported Models (via OpenRouter Proxy)

| Model | Use Case | Speed |
|-------|----------|-------|
| `moonshotai/kimi-k2.5-thinking` | Complex reasoning tasks | Medium |
| `moonshotai/kimi-k2.5` | Routine coding | Fast |
| `z-ai/glm-4.7` | Fast iteration | Fast |

### Environment (on sprites)

```bash
export ANTHROPIC_BASE_URL="https://openrouter.ai/api"
export ANTHROPIC_AUTH_TOKEN="$OPENROUTER_API_KEY"
export ANTHROPIC_MODEL="moonshotai/kimi-k2.5"
```

**NEVER set `ANTHROPIC_API_KEY` on sprites.** Risk of accidental billing.

## Sprite Lifecycle

1. **Spawn** — `sprite create <name>` (1-2 seconds)
2. **Bootstrap** — Clone repos, install deps, write env config
3. **Checkpoint** — `sprite-env checkpoints create` (instant)
4. **Task** — `claude --yolo` with full task description
5. **Collect** — Check git diff, push changes
6. **Checkpoint** — Save successful state
7. **Sleep** — Automatic after 30s idle (near-zero cost)
8. **Wake** — Instant when next task arrives

**Sprites are persistent.** Don't destroy them. They auto-sleep for free.

## Coding Standards

- **Go 1.23+**
- `gofmt` + `golangci-lint`
- Semantic commit messages: `feat:`, `fix:`, `test:`, `docs:`, `refactor:`
- Table-driven tests (see `internal/provider/provider_test.go` for examples)
- Handle errors explicitly — no `_` for error returns
- **No bash scripts** — write Go code for automation

## Important Rules

- **Sprites, not Machines.** Use `sprite` CLI, not `fly machines` or `flyctl`.
- **Claude Code, not OpenCode.** Claude Code is the canonical harness (see ADR-001). Use proxy provider for non-Anthropic models.
- **Persistent, not ephemeral.** Don't destroy sprites after tasks.
- **Checkpoint aggressively.** After bootstrap, after successful tasks, before risky operations.
- **Tests required** for all new functionality.
- **Write code within 5 minutes.** Don't spend time analyzing endlessly.
- **Commit early, commit often.** Small atomic commits with semantic messages.
