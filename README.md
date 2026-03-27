# Bitterblossom

Remote conductor and thin transport for a [Sprites](https://sprites.dev) software factory.

## What This Is

Bitterblossom has three surfaces:

- `conductor/`: Elixir/OTP orchestrator — leases issues, dispatches builds, governs PRs, merges
- `bb`: thin Go transport for sprite setup, dispatch, status, logs, and recovery
- `base/skills/`: skill library provisioned onto every managed sprite

Legacy shell and Python entrypoints have been retired. The supported control plane is the Elixir conductor under `conductor/`, with `bb` kept as the thin sprite transport.

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
scripts/                 prompt templates, onboarding helpers, runtime contract tests
sprites/                 per-sprite personas
docs/adr/                architecture decisions
docs/architecture/       system overview + per-module drill-downs
docs/                    operator docs and contracts
```

## How It Works

1. `bb setup <sprite> --repo owner/repo` bootstraps persistent worker sprites with base configs, imported autonomy skills, and a role persona
2. `cd conductor && mix conductor start --fleet ../fleet.toml` boots the Elixir control plane and starts leasing runnable issues
3. the conductor dispatches a builder sprite with a branch contract
4. the builder opens a PR on that branch; PR existence is the success signal
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

# 2) Log into Codex locally so the conductor can seed managed sprites.
# The file-backed auth cache is the default path; OPENAI_API_KEY remains
# a compatibility fallback when local Codex auth is unavailable.
mkdir -p "${CODEX_HOME:-$HOME/.codex}"
grep -q 'cli_auth_credentials_store = "file"' "${CODEX_HOME:-$HOME/.codex}/config.toml" 2>/dev/null || \
  printf '\ncli_auth_credentials_store = "file"\n' >> "${CODEX_HOME:-$HOME/.codex}/config.toml"
codex login

# 3) Reconcile the managed fleet
cd conductor
mix deps.get
mix compile
mix conductor fleet --fleet ../fleet.toml --reconcile

# 4) Start the conductor
mix conductor start --fleet ../fleet.toml
```

Use `cd conductor && mix conductor pause`, `cd conductor && mix conductor resume`, `cd conductor && mix conductor show-runs`, and `cd conductor && mix conductor show-events` to inspect or control the running pipeline.

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

Codex account auth is also never stored in git. The conductor seeds managed sprites from the local `${CODEX_HOME:-~/.codex}/auth.json` cache when it is available; treat that file like a password. Never commit it, paste it into tickets, or log its contents. If local Codex auth is unavailable, Bitterblossom falls back to `OPENAI_API_KEY` for Codex dispatches.

## CI Pipeline

GitHub Actions CI runs on pull requests and pushes to `master` with:
- `shellcheck` for `scripts/*.sh`
- `ruff` + `pytest` for `base/hooks/`
- `yamllint` for `compositions/`

## Repo Verification

Use `make test` as the supported repo-level verification command. It runs
hook tests and conductor tests from the repo root, including conductor
dependency bootstrap in the command path.

## Python Testing (Hooks + Runtime Contract)

Safety-critical hooks and the remaining Python runtime-contract checks are covered with pytest and ruff:

```bash
python3 -m pytest -q base/hooks/ scripts/test_runtime_contract.py
ruff check base/hooks
```

## Troubleshooting

### Dispatch blocked by a stale agent process

If a previous dispatch was interrupted (Ctrl-C, network drop, timeout), an agent process may still be running on the sprite. A live agent process blocks the next dispatch.

```bash
bb kill <sprite>
```

This terminates stale agent processes and clears the way for a fresh dispatch. Dispatch also performs a best-effort cleanup before it starts.

## Constraints

- PR review is a separate GitHub Action (multi-model council), not done by sprites
- Sprite GitHub identity is env-configurable (shared bot by default, per-sprite overrides supported)
- Human approval required for composition changes
- Fae/fairy naming convention throughout
