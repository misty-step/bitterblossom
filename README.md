# Bitterblossom

Remote conductor and thin transport for a [Sprites](https://sprites.dev) software factory.

## What This Is

Bitterblossom has three surfaces:

- `conductor/`: Elixir/OTP orchestrator — leases issues, dispatches builds, governs PRs, merges
- `bb`: thin Go transport for sprite setup, dispatch, status, logs, and recovery
- `base/skills/`: skill library provisioned onto every managed sprite

The Python conductor (`scripts/conductor.py`) is deprecated as of [ADR-004](docs/adr/004-elixir-conductor-architecture.md); it remains as reference only.

The design is intentional:

- `bb` stays deterministic and small
- the conductor owns workflow judgment and durable state
- GitHub is the human-facing work ledger
- SQLite + JSONL event logs are the machine-facing run ledger

Read [WORKFLOW.md](WORKFLOW.md) first for the repo-owned runtime workflow contract, then [ADR-002](docs/adr/002-architecture-minimalism.md) for the thin-CLI boundary and [ADR-003](docs/adr/003-conductor-control-plane.md) for the remote conductor design.

## Architecture

Full artifact stack: [docs/architecture/README.md](docs/architecture/README.md)

```text
conductor/               Elixir/OTP orchestrator (control plane)
cmd/bb/                  thin Go transport CLI (sprite edge)
base/skills/             skill files provisioned onto sprites
scripts/                 ralph loop + prompt templates + legacy Python conductor
sprites/                 per-sprite personas
docs/adr/                architecture decisions
docs/architecture/       system overview + per-module drill-downs
docs/                    operator docs and contracts
```

## How It Works

1. `bb setup <sprite> --repo owner/repo` bootstraps persistent worker sprites with base configs, imported autonomy skills, and a role persona
2. `scripts/conductor.py run-once|loop` reads GitHub issues and acquires a lease
3. the conductor dispatches a builder sprite with a branch + artifact contract
4. the builder opens a PR and writes `builder-result.json`
5. three reviewer sprites run adversarial reviews and write review artifacts
6. the conductor requests revisions until quorum passes
7. the conductor evaluates review and CI signals, merges when policy allows, and records the run

The default human workflow is not "dispatch ad hoc prompts forever." It is "operate the conductor, inspect runs, recover when needed."

## Workflow Contract

`WORKFLOW.md` is the primary agent-facing contract for Bitterblossom's runtime phases.

It defines:
- the canonical phase order (`shape -> build -> review -> fix -> merge -> recover`)
- the default worker mapping for those phases
- the required imported skills for each phase
- the policy distinction between semantic readiness, policy mergeability, and mechanical check state

If README prose, persona guidance, or prompt templates drift from that contract, the contract wins.

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

# 3) Run one conductor cycle against the backlog
python3 scripts/conductor.py run-once \
  --repo misty-step/bitterblossom \
  --worker noble-blue-serpent \
  --reviewer council-fern-20260306 \
  --reviewer council-sage-20260306 \
  --reviewer council-thorn-20260306
```

See [docs/CLI-REFERENCE.md](docs/CLI-REFERENCE.md) for `bb`, and [docs/CONDUCTOR.md](docs/CONDUCTOR.md) for the conductor loop.

## Agent Skills

Bitterblossom ships task-oriented skill files in `base/skills/` so agents can load workflow guidance directly instead of inferring from CLI help text.

Imported first-party autonomy skills (vendored from `phrazzld/agent-skills`):
- `autopilot`
- `shape`
- `build`
- `pr`
- `pr-walkthrough`
- `debug`
- `pr-fix`
- `pr-polish`

Bitterblossom-specific runtime skills:
- `base/skills/bitterblossom-dispatch/SKILL.md` — dry-run probing plus prompt dispatch through `bb`
- `base/skills/bitterblossom-monitoring/SKILL.md` — monitoring, status checks, and recovery triage

`bb setup` provisions the full `base/skills/` tree onto managed sprites so role personas can rely on a version-pinned skill surface, while repo clones provide the versioned `WORKFLOW.md` contract those skills are meant to follow.

Example workflow using the current transport surface:

```bash
bb dispatch bramble "dry-run readiness probe" --repo misty-step/bitterblossom --dry-run
bb dispatch bramble "Implement issue 252" --repo misty-step/bitterblossom
bb logs bramble --follow
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

## Python Testing (Hooks + Conductor)

Safety-critical hooks in `base/hooks/` and the conductor script are covered with pytest and ruff. Use the Makefile targets:

```bash
make test-python   # pytest: base/hooks + scripts/test_conductor.py
make lint-python   # ruff:   base/hooks + scripts/conductor.py + tests
```

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
