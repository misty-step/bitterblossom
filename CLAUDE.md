# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What This Is

Bitterblossom is a declarative sprite factory for provisioning and managing a fleet of Fly.io machines running Claude Code agents. It's the configuration layer for OpenClaw (Kaylee), a multi-agent coordinator that routes engineering tasks to specialized sprites.

This is **not a traditional codebase** — it's infrastructure-as-config for AI agent orchestration. There are no build steps, no tests, no dependencies to install.

## Architecture

```
bitterblossom/
├── base/              # Shared config inherited by ALL sprites
│   ├── CLAUDE.md      # Engineering philosophy (sprite prompt)
│   ├── settings.json  # Claude Code hooks config
│   ├── hooks/         # Python safety hooks (PreToolUse, PostToolUse, Stop)
│   ├── skills/        # Reference skills (testing, naming, git, integration)
│   └── commands/      # Workflow commands (commit)
├── compositions/      # Team hypotheses as YAML — defines the fleet
│   └── v1.yaml        # Active: 5 sprites, keyword routing, fallback to bramblecap
├── sprites/           # Individual sprite identity + specialization prompts
├── observations/      # Kaylee's learning journal (manual pattern logging)
├── scripts/           # Lifecycle: provision.sh, sync.sh, teardown.sh
└── openclaw/          # Routing config (agents.yaml) + integration docs
```

**Key relationships:**
- `compositions/v1.yaml` is the source of truth for fleet configuration
- `openclaw/agents.yaml` mirrors routing from the composition for Kaylee's runtime use
- `base/` contents get copied to every sprite machine during provisioning
- Each sprite gets `base/` + its own `sprites/<name>.md` definition

## Lifecycle Scripts

```bash
./scripts/provision.sh bramblecap    # Create a Fly.io machine for a sprite
./scripts/provision.sh --all         # Provision entire fleet
./scripts/sync.sh                    # Push config updates to running sprites
./scripts/sync.sh bramblecap         # Sync specific sprite
./scripts/teardown.sh bramblecap     # Decommission (exports MEMORY.md first)
```

All scripts are **placeholder implementations** (TODO: Fly.io API calls). Structure and validation logic exists; actual machine operations don't yet.

## Sprites (The Fleet)

5 full-stack agents with specialization preferences. Routing is advisory, not restrictive.

| Sprite | Preference | Fallback |
|--------|-----------|----------|
| Bramblecap | Systems & Data | Default for ambiguous tasks |
| Willowwisp | Interface & Experience | |
| Thornguard | Quality & Security | |
| Fernweaver | Platform & Operations | |
| Mosshollow | Architecture & Evolution | |

## Hooks System

`base/settings.json` wires Python hooks into Claude Code's lifecycle:

| Hook | Trigger | Purpose |
|------|---------|---------|
| `destructive-command-guard.py` | PreToolUse (Bash) | Blocks `rm`, force push, `git reset --hard`, direct push to main |
| `github-cli-guard.py` | PreToolUse (Bash) | Transforms `gh issue view` to avoid deprecated `projectCards` field |
| `fast-feedback.py` | PostToolUse (Edit/Write) | Auto-runs type checker after file edits (detects TS/Python/Rust/Go) |
| `memory-reminder.py` | Stop | Prompts sprite to update MEMORY.md before session ends |

Hooks target **Linux** (Fly.io machines), not macOS. The destructive-command-guard uses `trash-cli` not `/usr/bin/trash`.

## Routing Algorithm

1. Extract keywords from task description
2. Score each sprite by keyword overlap (defined in `openclaw/agents.yaml`)
3. Apply rule overrides (e.g., "bug" → Thornguard, "deploy" → Fernweaver)
4. Highest score wins; ties use preference match; fallback = Bramblecap

## Key Constraints

- Fae/fairy naming convention for all sprites and concepts
- All sprites share a single GitHub account
- PR review is a separate GitHub Action (multi-model council), not done by sprites
- Human approval required for composition changes
- Compositions are hypotheses — designed to be cheap to change and iterate on
