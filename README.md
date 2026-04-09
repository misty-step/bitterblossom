# Bitterblossom

Bitterblossom is an Elixir infrastructure layer for an agent-run software
factory. It reconciles a fleet of sprites, supports a local-first workflow, and
exposes operator surfaces for truthful fleet inspection and per-sprite lifecycle
control.

## Supported Surfaces

- `conductor/`: Elixir/OTP control plane and operator commands
- `base/`: shared runtime config, hooks, and skills uploaded to managed sprites
- `sprites/`: repo-owned agent personas and loop instructions

Historical note: [`cmd/bb/`](cmd/bb/) still exists in the repo, but it is a
legacy surface pending #703. Root docs should treat the conductor as the
supported path.

## What Runs Today

1. Load `fleet.toml`
2. Reconcile the declared sprites
3. Start the declared loops for that fleet file
4. Let each sprite observe local repo state, Canary state, and conductor
   evidence through repo-owned skills
5. Inspect health, logs, and events through conductor commands

Read [WORKFLOW.md](WORKFLOW.md) first for the runtime contract, then
[project.md](project.md) for current product intent and
[docs/CONDUCTOR.md](docs/CONDUCTOR.md) for the operator surface.

## Quick Start

```bash
source .env.bb

mkdir -p "${CODEX_HOME:-$HOME/.codex}"
grep -q 'cli_auth_credentials_store = "file"' "${CODEX_HOME:-$HOME/.codex}/config.toml" 2>/dev/null || \
  printf '\ncli_auth_credentials_store = "file"\n' >> "${CODEX_HOME:-$HOME/.codex}/config.toml"
codex login

make ci-fast

cd conductor
mix deps.get
mix compile
mix conductor fleet --fleet ../fleet.toml --reconcile
mix conductor start --fleet ../fleet.toml
```

Use `mix conductor fleet`, `mix conductor logs <sprite>`,
`mix conductor show-events --limit 50`, and `mix conductor dashboard` to inspect
the live system.

For direct sprite lifecycle control on declared fleet entries:

```bash
cd conductor
mix conductor fleet audit --fleet ../fleet.toml
mix conductor sprite status bb-tansy --fleet ../fleet.toml --json
mix conductor sprite start bb-tansy --fleet ../fleet.toml
mix conductor sprite pause bb-tansy --fleet ../fleet.toml --wait
mix conductor sprite resume bb-tansy --fleet ../fleet.toml
mix conductor sprite stop bb-tansy --fleet ../fleet.toml
```

For local landing:

```bash
make ci
scripts/land.sh feat/example-change --delete-branch
```

## Repo Layout

```text
conductor/               Elixir/OTP control plane
base/                    shared hooks, settings, and skills for sprites
sprites/                 repo-owned personas and loop definitions
backlog.d/               shaped local backlog items
docs/                    operator docs, architecture notes, audits
.evidence/               local review and verification artifacts
```

See [docs/CODEBASE_MAP.md](docs/CODEBASE_MAP.md) for a deeper map of the
current codebase.

## Fleet Model

The default `fleet.toml` declares:

- `bb-tansy` running the Tansy loop

The conductor provisions and manages only the sprites named in the selected
fleet file. The tracked root fleet is the minimal responder fleet. Alternate
dev fleets can declare builder, fixer, polisher, or muse lanes when needed.
Repo assignment, clone transport, default branch, model, reasoning effort, and
persona prompt come from that file.

## Runtime Profile

Managed sprites use the stable profile alias `sonnet` from
`base/settings.json`. The current canonical OpenRouter Claude model identifier
behind that profile is `anthropic/claude-sonnet-4-6`, as configured in
`scripts/lib.sh`.

## Skills And Personas

Managed sprites receive:

- repo-owned loop instructions from `sprites/`
- shared runtime guidance from [WORKFLOW.md](WORKFLOW.md), [AGENTS.md](AGENTS.md),
  and [project.md](project.md)
- uploaded skill packs from `base/` and the configured spellbook

This keeps judgment in the agent layer while the conductor stays responsible
for provisioning, dispatch, health, and operator visibility.

## Security

Secret detection runs in the local Dagger verification flow. Treat `make ci` as
part of the landing path, not an optional pre-push nicety. See
[docs/SECRETS.md](docs/SECRETS.md) for local usage, leak response, and sprite
auth handling.

API keys and Codex auth caches are never meant to live in git. Treat
`${CODEX_HOME:-~/.codex}/auth.json` like a password.

## Verification

Dagger is the supported repo-level verification surface:

- `make ci-fast` for the fast lane
- `make ci` for the full lane
- `scripts/land.sh <branch>` for verdict validation, full verification, and
  local squash landing

`scripts/land.sh` requires a fresh `ship` verdict for the branch tip. Conditional
or stale verdicts must be refreshed before landing.

Verdict refs live under `refs/verdicts/<branch>`. Evidence bundles live under
`.evidence/<branch>/<date>/`.

## Repo Verification

Use `make test` as the supported repo-level verification command. It currently
aliases `make ci`, so the root verification path still runs the local Dagger
lane and stays compatible with older runtime-contract checks.

`scripts/ci/dagger-call.sh` assumes a trusted machine because Dagger still uses
a privileged engine. If you intentionally run it inside CI, set
`BB_ALLOW_PRIVILEGED_DAGGER_IN_CI=1` on that runner.

## Troubleshooting

If fleet repair or dispatch behaves unexpectedly, start from:

```bash
make ci-fast

cd conductor
mix conductor fleet --fleet ../fleet.toml --reconcile
mix conductor logs bb-tansy --lines 100
```

For current command details, see [docs/CLI-REFERENCE.md](docs/CLI-REFERENCE.md).
