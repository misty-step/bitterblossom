# Bitterblossom

Declarative sprite factory for provisioning and managing a fleet of Fly.io Sprites running Claude Code.

## What This Is

Bitterblossom is how OpenClaw (Kaylee) provisions, manages, experiments with, and iterates on sprite team compositions. Compositions are **hypotheses** — cheap to change, designed to be tested. The goal is continuous experimentation: what configurations are most effective, which specializations matter, what's most productive.

**v1:** Declarative config + real provisioning + observation journal + experimentation infrastructure.

## Architecture

```
bitterblossom/
├── cmd/bb/                # Go CLI control plane
├── internal/              # Core packages (dispatch, watchdog, agent, lifecycle, fleet)
├── pkg/                   # Shared libraries (fly, events)
├── base/                  # Shared config all sprites inherit
│   ├── CLAUDE.md          # Base engineering philosophy (prompt)
│   ├── settings.json      # Claude Code config (env, hooks)
│   ├── hooks/             # Safety hooks (git guards, fast-feedback)
│   ├── skills/            # Portable reference skills
│   └── commands/          # Workflow commands (commit)
├── compositions/          # Team hypotheses (YAML)
│   ├── v1.yaml            # Current active composition
│   └── archive/           # Previous compositions for comparison
├── sprites/               # Individual sprite identity + persona
├── observations/          # Learning journal + experiment results
│   └── archives/          # Exported data from decommissioned sprites
├── scripts/               # Legacy shell scripts (deprecated, see docs/MIGRATION.md)
├── docs/                  # CLI reference, contracts, migration guide
└── openclaw/              # Integration docs for Kaylee
```

## How It Works

1. **Provision:** `bb provision bramble` — creates a Fly.io sprite, uploads base config + persona, configures Claude Code
2. **Dispatch:** `bb dispatch bramble "Implement the auth middleware" --execute` — sends work to a sprite
3. **Monitor:** `bb watchdog` — check fleet health, auto-recover dead sprites
4. **Observe:** After tasks complete, log patterns in `observations/OBSERVATIONS.md`
5. **Iterate:** Edit composition YAML, `bb compose apply --execute`, observe again
6. **Experiment:** Try different compositions, compare observations, evolve

## Quick Start

```bash
# Required for provision/sync (settings.json is rendered at runtime)
export ANTHROPIC_AUTH_TOKEN="<moonshot-key>"
export FLY_APP="<fly-app-name>"
export FLY_API_TOKEN="<fly-api-token>"

# Recommended for GitHub permission isolation (phase 1 shared bot account)
export SPRITE_GITHUB_DEFAULT_USER="misty-step-sprites"
export SPRITE_GITHUB_DEFAULT_EMAIL="misty-step-sprites@users.noreply.github.com"
export SPRITE_GITHUB_DEFAULT_TOKEN="<github-bot-token>"

# Fleet status
bb status --format text

# Composition health: desired vs actual
bb compose status

# Provision all sprites from current composition
bb provision --all

# Provision a single sprite
bb provision bramble

# Dispatch a task (dry-run first, then execute)
bb dispatch bramble "Build the user authentication API"
bb dispatch bramble "Build the user authentication API" --execute

# Dispatch with repo clone
bb dispatch thorn --repo misty-step/heartbeat "Write tests for the webhook handler" --execute

# Fleet health check (identifies dead/stale/blocked sprites)
bb watchdog
bb watchdog --execute    # auto-redispatch dead sprites

# Sync config updates to running fleet
bb sync

# Decommission a sprite (exports MEMORY.md first)
bb teardown bramble
```

See [docs/CLI-REFERENCE.md](docs/CLI-REFERENCE.md) for the complete command reference.

## Multi-Provider Support (New in v3)

Bitterblossom now supports multiple LLM providers. You can configure different providers per sprite:

```bash
# Provision a sprite with Claude via OpenRouter
export BB_PROVIDER_HEMLOCK=openrouter-claude
export BB_MODEL_HEMLOCK=anthropic/claude-opus-4
bb provision hemlock
```

**Supported providers:**
- `moonshot-anthropic` — Moonshot Anthropic endpoint (preferred for Claude Code) [default]
- `moonshot` — Native Moonshot AI API (Kimi models)
- `openrouter-kimi` — Kimi models via OpenRouter
- `openrouter-claude` — Claude models via OpenRouter

See [docs/PROVIDERS.md](docs/PROVIDERS.md) for full documentation and [compositions/v3-multi-provider.yaml](compositions/v3-multi-provider.yaml) for an example configuration.

## Composition Philosophy

Compositions are hypotheses. The current v1 is 5 full-stack sprites with specialization preferences. OpenClaw (Kaylee) decides where to route work — routing is intelligent, not programmatic.

| Sprite | Preference | Think of them as |
|--------|-----------|-----------------|
| **Bramble** | Systems & Data | The backend engineer |
| **Willow** | Interface & Experience | The frontend craftsperson |
| **Thorn** | Quality & Security | The security-minded tester |
| **Fern** | Platform & Operations | The DevOps specialist |
| **Moss** | Architecture & Evolution | The tech lead / architect |

Any sprite can handle any task. Preferences improve quality for domain work but never block progress.

## Fleet Management: Sprites

**Important:** This uses Fly.io **Sprites**, not Machines. They are different products:
- Sprites: AI-native workloads with durable 100GB disks, auto-sleep (~$0 idle), Claude Code pre-installed
- Machines: General-purpose VMs

Always use the `sprite` CLI, not `fly machines`.

### API Endpoint

Sprites are managed through the `api.sprites.dev` endpoint:
```bash
export FLY_API_HOSTNAME="https://api.sprites.dev"
```

### Sprite CLI Commands

The sprite CLI provides specialized commands for managing AI workloads:

```bash
# Create a new sprite
sprite create <name> --region <region>

# List all sprites
sprite list

# Get sprite details
sprite show <name>

# Connect to a sprite (SSH access)
sprite connect <name>

# Destroy a sprite
sprite destroy <name>
```

### Sprites vs Machines Comparison

| Feature | Sprites | Machines |
|---------|---------|----------|
| **Purpose** | AI-native workloads | General-purpose VMs |
| **Disk** | 100GB durable disk | Variable, ephemeral by default |
| **Auto-sleep** | Yes (~$0 when idle) | No (always running) |
| **Claude Code** | Pre-installed | Manual installation |
| **Cost model** | Sleep-based billing | Always-on billing |
| **CLI** | `sprite` commands | `fly machines` commands |
| **API endpoint** | api.sprites.dev | api.fly.io |

## Experimentation

The observation journal (`observations/OBSERVATIONS.md`) is the core feedback loop. Every meaningful task result gets logged. Over time, patterns emerge:
- Which sprites handle which task types best
- Whether specialization actually helps vs generalist sprites
- What base config changes improve quality across the board
- Whether 5 sprites is too many, too few, or just right

Compositions live in `compositions/`. When patterns suggest a change, create a new composition version, provision it, and compare.

## Security

Secret detection runs on every PR and push to master via [TruffleHog](https://github.com/trufflesecurity/trufflehog). See [docs/SECRETS.md](docs/SECRETS.md) for local usage, leak response runbook, and how sprite auth tokens work.

API keys are never stored in git. `base/settings.json` uses a placeholder rendered at provision/sync time from `$ANTHROPIC_AUTH_TOKEN`.

## CI Pipeline

GitHub Actions CI runs on pull requests and pushes to `master` with:
- `shellcheck` for `scripts/*.sh`
- `ruff` + `pytest` for `base/hooks/`
- `yamllint` for `compositions/`

## Hook Testing

Safety-critical hooks in `base/hooks/` are covered with pytest:

```bash
python3 -m pytest -q
```

## Constraints

- PR review is a separate GitHub Action (multi-model council), not done by sprites
- Sprite GitHub identity is env-configurable (shared bot by default, per-sprite overrides supported)
- Human approval required for composition changes
- Fae/fairy naming convention throughout
