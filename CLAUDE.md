# CLAUDE.md

This file provides guidance to Claude Code when working in this repository.

## What This Is

Bitterblossom is a declarative sprite factory. It provisions and manages Fly.io Sprites running Claude Code. This is infrastructure-as-config for AI agent orchestration.

**No build steps, no tests, no dependencies.** Just config, scripts, and documentation.

## Key Concepts

- **Sprites** = Fly.io AI workloads (NOT Machines). Durable 100GB disk, auto-sleep, Claude Code pre-installed.
- **Compositions** = Team hypotheses. YAML files defining which sprites exist and their preferences.
- **Base config** = Shared engineering philosophy, hooks, skills inherited by all sprites.
- **Persona** = Individual sprite identity and specialization (in `sprites/`).
- **Observations** = Learning journal tracking what works and what doesn't.

## Architecture

```
base/              → Shared config pushed to every sprite
compositions/      → Team hypotheses (YAML)
sprites/           → Individual persona definitions
observations/      → Learning journal + experiment archives
scripts/           → Real lifecycle scripts (provision, sync, teardown, dispatch, status)
openclaw/          → Integration docs
```

## Scripts

All scripts are real implementations using the `sprite` CLI:

| Script | Purpose |
|--------|---------|
| `lib.sh` | Shared functions (sourced by other scripts) |
| `provision.sh` | Create sprite + upload config |
| `sync.sh` | Push config updates to running sprites |
| `teardown.sh` | Export data + destroy sprite |
| `dispatch.sh` | Send a task to a sprite (one-shot or Ralph loop) |
| `status.sh` | Fleet overview |

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
