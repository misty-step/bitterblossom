# CLAUDE.md

This file provides guidance to Claude Code when working in this repository.

## What This Is

Bitterblossom is a declarative sprite factory. It provisions and manages Fly.io Sprites running Claude Code. This is infrastructure-as-config for AI agent orchestration.

Go control plane (`bb` CLI) for fleet lifecycle, dispatch, and monitoring. Config, personas, and compositions are declarative.

## Key Concepts

- **Sprites** = Fly.io AI workloads (NOT Machines). Durable 100GB disk, auto-sleep, Claude Code pre-installed.
- **Compositions** = Team hypotheses. YAML files defining which sprites exist and their preferences.
- **Base config** = Shared engineering philosophy, hooks, skills inherited by all sprites.
- **Persona** = Individual sprite identity and specialization (in `sprites/`).
- **Observations** = Learning journal tracking what works and what doesn't.

## Architecture

```
cmd/bb/            → Go CLI control plane (Cobra)
internal/          → Core packages (dispatch, watchdog, agent, lifecycle, fleet)
pkg/               → Shared libraries (fly, events)
base/              → Shared config pushed to every sprite
compositions/      → Team hypotheses (YAML)
sprites/           → Individual persona definitions
observations/      → Learning journal + experiment archives
scripts/           → Legacy shell scripts (deprecated, see docs/MIGRATION.md)
openclaw/          → Integration docs
docs/              → CLI reference, contracts, migration guide
```

## CLI

Primary interface is `bb` (Go CLI). See `docs/CLI-REFERENCE.md` for full reference.

| Command | Purpose |
|---------|---------|
| `bb provision` | Create sprite + upload config |
| `bb sync` | Push config updates to running sprites |
| `bb teardown` | Export data + destroy sprite |
| `bb dispatch` | Send a task to a sprite (one-shot or Ralph loop) |
| `bb status` | Fleet overview or sprite detail |
| `bb watchdog` | Fleet health checks + auto-recovery |
| `bb compose` | Composition-driven reconciliation |
| `bb agent` | On-sprite supervisor (start, stop, status, logs) |
| `bb watch` | Real-time event stream dashboard |
| `bb logs` | Historical event log queries |

Build: `go build -o bb ./cmd/bb`

## Hooks

Three hooks in `base/hooks/`:

| Hook | Trigger | Purpose |
|------|---------|---------|
| `destructive-command-guard.py` | PreToolUse (Bash) | Blocks destructive git ops (force push, direct push to main, rebase, --no-verify) |
| `fast-feedback.py` | PostToolUse (Edit/Write) | Auto-runs type checker after file edits |
| `memory-reminder.py` | Stop | Prompts sprite to update MEMORY.md |

**Note:** `rm` is allowed — sprites run on disposable machines. Only git operations are guarded because they affect shared remote state.

## Important

- **Sprites, not Machines.** Use `sprite` CLI, not `fly machines`.
- **Compositions are hypotheses.** Designed to be cheap to change and test.
- **OpenClaw routes intelligently.** No programmatic keyword matching — Kaylee decides.
- **Observation journal is mandatory.** Every experiment needs data.
