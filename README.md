# Bitterblossom

Declarative sprite factory for provisioning and managing a fleet of [Sprites](https://sprites.dev) running Claude Code.

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

1. **Provision:** `bb provision bramble` — creates a sprite, uploads base config + persona, configures Claude Code
2. **Dispatch:** `bb dispatch bramble "Implement the auth middleware" --execute` — sends work to a sprite
3. **Monitor:** `bb watchdog` — check fleet health, auto-recover dead sprites
4. **Fleet:** `bb fleet` — view registered sprites and reconcile fleet state
5. **Observe:** After tasks complete, log patterns in `observations/OBSERVATIONS.md`
6. **Iterate:** Edit composition YAML, `bb compose apply --execute`, observe again
7. **Experiment:** Try different compositions, compare observations, evolve

## Quick Start

```bash
# 0) Build CLI
go build -o bb ./cmd/bb

# 1) Generate env exports (auto-detects org/app where possible)
./scripts/onboard.sh --app bitterblossom-dash --write .env.bb
source .env.bb

# 2) If FLY_API_TOKEN is empty in .env.bb, create one (fly auth token is deprecated)
fly tokens create org -o misty-step -n bb-cli -x 720h
# then paste token into .env.bb and source again

# 3) Set auth key
export OPENROUTER_API_KEY="<openrouter-key>"

# 3.1) Required for Cerberus PR review (GitHub Actions)
printf '%s' "$OPENROUTER_API_KEY" | gh secret set OPENROUTER_API_KEY --repo misty-step/bitterblossom

# 4) Launch a Ralph loop
./bb provision bramble
./bb dispatch bramble --repo misty-step/bitterblossom --ralph --file /tmp/task.md --execute
./bb watchdog --sprite bramble
```

See [docs/CLI-REFERENCE.md](docs/CLI-REFERENCE.md) for the complete command reference.

## Agent Skills

Bitterblossom ships task-oriented skill files in `base/skills/` so agents can load workflow guidance directly instead of inferring from CLI help text.

Current Bitterblossom-specific skills:
- `base/skills/bitterblossom-dispatch/SKILL.md` — dispatching issues/tasks with mounted skills
- `base/skills/bitterblossom-monitoring/SKILL.md` — monitoring, status checks, and recovery triage

Example using explicit skill mounting during dispatch:

```bash
bb dispatch bramble --issue 252 --repo misty-step/bitterblossom \
  --skill base/skills/bitterblossom-dispatch \
  --skill base/skills/bitterblossom-monitoring \
  --execute --wait
```

## Runtime Profile

Bitterblossom ships one canonical runtime profile out of the box:

- Provider: `openrouter-claude`
- Model: `anthropic/claude-sonnet-4-6`
- Plugin: `ralph-loop@claude-plugins-official`
- Auth: `OPENROUTER_API_KEY`

Legacy provider variants are still parseable for compatibility, but they are not the default path. See [docs/PROVIDERS.md](docs/PROVIDERS.md) for compatibility notes.

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

**Important:** [Sprites](https://sprites.dev) are a standalone service — they are NOT Fly.io Machines. Sprites are isolated Linux sandboxes with persistent filesystems, purpose-built for AI workloads. Always use the `sprite` CLI, never `fly machines`.

### API & CLI

- **API:** `https://api.sprites.dev` ([docs](https://sprites.dev/api))
- **CLI:** `sprite` (installed at `~/.local/bin/sprite`)
- **SDKs:** Go, Node, Python, Elixir

```bash
sprite create <name>          # Create a new sprite
sprite list                   # List all sprites
sprite show <name>            # Get sprite details
sprite connect <name>         # SSH access
sprite destroy <name>         # Destroy a sprite
```

### Key Properties

| Property | Detail |
|----------|--------|
| **Disk** | 100GB persistent filesystem |
| **Auto-sleep** | Yes (~$0 when idle) |
| **Claude Code** | Pre-installed |
| **Checkpoints** | Point-in-time snapshots and restore |
| **Networking** | TCP proxy tunneling, DNS-based network policies |
| **Exec** | WebSocket-based command execution |

## Fleet Management: bb fleet

The `bb fleet` command provides visibility into your registered sprites and supports reconciliation with Fly.io.

### List Fleet Status

```bash
bb fleet                          # Show all registered sprites with status
bb fleet --format json            # Machine-readable output
```

Shows:
- All sprites from the registry (~/.config/bb/registry.toml)
- Current status (running, not found, orphaned)
- Machine IDs and creation time
- Orphaned sprites (exist in Fly.io but not registered)

### Sync Fleet State

```bash
bb fleet --sync                   # Create missing sprites from registry
bb fleet --sync --dry-run         # Preview what would be created
```

Creates sprites that are registered but don't exist in Fly.io. Uses the standard provisioning flow (base config + persona + bootstrap).

### Prune Orphaned Sprites

```bash
bb fleet --sync --prune           # Remove sprites not in registry
bb fleet --sync --prune --dry-run # Preview what would be destroyed
```

Destroys sprites that exist in Fly.io but aren't in the registry. **Requires confirmation** unless using `--dry-run`. Archives observations before destruction.

### Examples

```bash
# Check fleet health
bb fleet

# Reconcile and create missing sprites
bb fleet --sync

# Full reconciliation with pruning (interactive)
bb fleet --sync --prune

# Preview destructive operations
bb fleet --sync --prune --dry-run
```

## Experimentation

The observation journal (`observations/OBSERVATIONS.md`) is the core feedback loop. Every meaningful task result gets logged. Over time, patterns emerge:
- Which sprites handle which task types best
- Whether specialization actually helps vs generalist sprites
- What base config changes improve quality across the board
- Whether 5 sprites is too many, too few, or just right

Compositions live in `compositions/`. When patterns suggest a change, create a new composition version, provision it, and compare.

## Security

Secret detection runs on every PR and push to master via [TruffleHog](https://github.com/trufflesecurity/trufflehog). See [docs/SECRETS.md](docs/SECRETS.md) for local usage, leak response runbook, and how sprite auth tokens work.

API keys are never stored in git. `base/settings.json` uses a placeholder rendered at provision/sync time from `$OPENROUTER_API_KEY` (with `$ANTHROPIC_AUTH_TOKEN` accepted as a legacy fallback).

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
