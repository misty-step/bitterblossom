# Bitterblossom

Bitterblossom is an Elixir conductor for an agent-run software factory. It reconciles a fleet of sprites, dispatches their autonomous loops, and exposes truthful operator surfaces for health, logs, and run state.

## Supported Surfaces

- `conductor/`: Elixir/OTP control plane and operator commands
- `base/`: shared runtime config, hooks, and skills uploaded to managed sprites
- `sprites/`: repo-owned agent personas and loop instructions

Historical note: [`cmd/bb/`](cmd/bb/) still exists in the repo, but it is legacy surface pending #703. Root docs should treat the conductor as the supported path.

## What Runs Today

1. Load `fleet.toml`
2. Reconcile the declared sprites
3. Start the builder/fixer/polisher loops on their assigned repos
4. Let each sprite observe GitHub state and act through its repo-owned skills
5. Inspect health, logs, and events through conductor commands

Read [WORKFLOW.md](WORKFLOW.md) first for the runtime contract, then [project.md](project.md) for current product intent and [docs/CONDUCTOR.md](docs/CONDUCTOR.md) for the operator surface.

## Quick Start

```bash
source .env.bb
export GITHUB_TOKEN="$(gh auth token)"

mkdir -p "${CODEX_HOME:-$HOME/.codex}"
grep -q 'cli_auth_credentials_store = "file"' "${CODEX_HOME:-$HOME/.codex}/config.toml" 2>/dev/null || \
  printf '\ncli_auth_credentials_store = "file"\n' >> "${CODEX_HOME:-$HOME/.codex}/config.toml"
codex login

cd conductor
mix deps.get
mix compile
mix conductor check-env
mix conductor fleet --fleet ../fleet.toml --reconcile
mix conductor start --fleet ../fleet.toml
```

Use `mix conductor fleet`, `mix conductor logs <sprite>`, `mix conductor show-events --run_id <id>`, and `mix conductor dashboard` to inspect the live system.

## Repo Layout

```text
conductor/               Elixir/OTP control plane
base/                    shared hooks, settings, and skills for sprites
sprites/                 repo-owned personas and loop definitions
backlog.d/               shaped local backlog items
docs/                    operator docs, architecture notes, audits
```

See [docs/CODEBASE_MAP.md](docs/CODEBASE_MAP.md) for a deeper map of the current codebase.

## Fleet Model

The default `fleet.toml` declares:

- `bb-builder` running the Weaver loop
- `bb-fixer` running the Thorn loop
- `bb-polisher` running the Fern loop

The conductor provisions only the sprites named in `fleet.toml`. Repo assignment, model, reasoning effort, and persona prompt come from that file.

## Runtime Profile

Managed sprites use the stable profile alias `sonnet` from `base/settings.json`.
The current canonical OpenRouter Claude model identifier behind that profile is
`anthropic/claude-sonnet-4-6`, as configured in `scripts/lib.sh`.

## Skills and Personas

Managed sprites receive:

- repo-owned loop instructions from `sprites/`
- shared runtime guidance from [WORKFLOW.md](WORKFLOW.md), [AGENTS.md](AGENTS.md), and [project.md](project.md)
- uploaded skill packs from `base/` and the configured spellbook

This keeps judgment in the agent layer while the conductor stays responsible for provisioning, dispatch, health, and operator visibility.

## Security

Secret detection runs on every PR and push to `master` via [TruffleHog](https://github.com/trufflesecurity/trufflehog). See [docs/SECRETS.md](docs/SECRETS.md) for local usage, leak response, and sprite auth handling.

API keys and Codex auth caches are never meant to live in git. Treat `${CODEX_HOME:-~/.codex}/auth.json` like a password.

## CI Pipeline

GitHub Actions CI runs on pull requests and pushes to `master` with:

- `shellcheck` for `scripts/*.sh`
- `ruff` + `pytest` for `base/hooks/`
- conductor tests via `make test`

## Repo Verification

Use `make test` as the supported repo-level verification command. It runs the hook tests and the conductor test suite from the repo root, including conductor dependency bootstrap in the command path.

## Troubleshooting

If fleet repair or dispatch behaves unexpectedly, start from:

```bash
cd conductor
mix conductor check-env
mix conductor fleet --fleet ../fleet.toml --reconcile
mix conductor logs bb-builder --lines 100
```

For current command details, see [docs/CLI-REFERENCE.md](docs/CLI-REFERENCE.md).
