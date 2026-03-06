# Bitterblossom

Remote conductor and thin transport for a [Sprites](https://sprites.dev) software factory.

## What This Is

Bitterblossom has two surfaces:

- `bb`: thin Go transport for sprite setup, dispatch, status, logs, and recovery
- `scripts/conductor.py`: remote control plane that leases GitHub issues, dispatches builders, runs a review council, waits for CI, and merges

The design is intentional:

- `bb` stays deterministic and small
- the conductor owns workflow judgment and durable state
- GitHub is the human-facing work ledger
- SQLite + JSONL event logs are the machine-facing run ledger

Read [ADR-002](docs/adr/002-architecture-minimalism.md) for the thin-CLI boundary and [ADR-003](docs/adr/003-conductor-control-plane.md) for the remote conductor design.

## Architecture

```text
cmd/bb/                  thin sprite transport CLI
scripts/conductor.py     conductor MVP with SQLite run store
scripts/prompts/         builder + reviewer prompt templates
base/                    shared CLAUDE/settings/hooks/skills pushed to sprites
sprites/                 per-sprite personas
docs/adr/                architecture decisions
docs/                    operator docs and contracts
```

## How It Works

1. `bb setup <sprite> --repo owner/repo` bootstraps persistent worker sprites
2. `scripts/conductor.py run-once|loop` reads GitHub issues and acquires a lease
3. the conductor dispatches a builder sprite with a branch + artifact contract
4. the builder opens a PR and writes `builder-result.json`
5. three reviewer sprites run adversarial reviews and write review artifacts
6. the conductor requests revisions until quorum passes
7. the conductor waits for green CI, satisfies merge policy, merges, and records the run

The default human workflow is not "dispatch ad hoc prompts forever." It is "operate the conductor, inspect runs, recover when needed."

## Quick Start

```bash
# 0) Build bb
make build

# 1) Load env
source .env.bb
export GITHUB_TOKEN="$(gh auth token)"
export OPENROUTER_API_KEY="..."

# Prefer SPRITE_TOKEN for local auth. It connects directly to Sprites,
# avoiding token exchange failures.
# FLY_API_TOKEN is a more fragile fallback.
export SPRITE_TOKEN="..."  # from https://sprites.dev/settings

# 2) Bootstrap one builder + three reviewers
./bin/bb setup noble-blue-serpent --repo misty-step/bitterblossom
./bin/bb setup council-fern-20260306 --repo misty-step/bitterblossom
./bin/bb setup council-sage-20260306 --repo misty-step/bitterblossom
./bin/bb setup council-thorn-20260306 --repo misty-step/bitterblossom

# 3) Run one conductor cycle against an issue or backlog label
python3 scripts/conductor.py run-once \
  --repo misty-step/bitterblossom \
  --label autopilot \
  --worker noble-blue-serpent \
  --reviewer council-fern-20260306 \
  --reviewer council-sage-20260306 \
  --reviewer council-thorn-20260306
```

See [docs/CLI-REFERENCE.md](docs/CLI-REFERENCE.md) for `bb`, and [docs/CONDUCTOR.md](docs/CONDUCTOR.md) for the conductor loop.

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

## Operating Model

- Workers are persistent sprites with warm repos and toolchains.
- Runs are tracked by `run_id` in `.bb/conductor.db`.
- Builder and reviewer artifacts live under `.bb/conductor/<run_id>/`.
- GitHub issues are the intake queue.
- GitHub PRs are the merge surface.
- `bb kill <sprite>` is the recovery path when a sprite gets stuck.

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

## Python Tests and Lint

Conductor operators can run the Python test suite and linter with two make targets:

```bash
make test-python   # pytest: base/hooks + scripts/test_conductor.py
make lint-python   # ruff: base/hooks + scripts/conductor.py + tests
```

These cover safety-critical hooks and the conductor itself. Run both before pushing conductor changes.

## Troubleshooting

### Dispatch blocked by a stale Ralph loop

If a previous dispatch was interrupted (Ctrl-C, network drop, timeout), the Ralph loop may still be running on the sprite. A live Ralph process blocks the next dispatch.

```bash
bb kill <sprite>
```

This terminates the Ralph loop and any associated agent processes, clearing the way for a fresh dispatch. Stale Claude-only processes (no active Ralph loop) are cleaned automatically by dispatch and don't require `bb kill`.

## Constraints

- PR review is a separate GitHub Action (multi-model council), not done by sprites
- Sprite GitHub identity is env-configurable (shared bot by default, per-sprite overrides supported)
- Human approval required for composition changes
- Fae/fairy naming convention throughout
