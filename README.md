# Bitterblossom

The event plane for agent workloads. Define a **task**, bind an **agent**,
attach a **trigger** — all as files — and the plane runs it durably on a
remote substrate (Fly Sprites today) with cost, budget, and trace visible
from the CLI.

Two kinds of work, named so we can talk about them:

- **Reflex** work — standing, trigger-fired (webhook/cron). The plane
  reacts without judgment, on cheap open-weight models, hermetically.
- **Dispatch** work — deliberate, operator- or agent-initiated from a
  terminal (`bb run`). May run as the operator on subscription auth.

```
plane.toml                  # db path, ingress bind, notify webhook, global budget
agents/<name>.toml          # versioned agent binding: harness, model, secrets
tasks/<name>/card.md        # lane card — the agent's entire context
tasks/<name>/task.toml      # agent, substrate, workspace, budgets, triggers
```

Those files are the operator's instance plane. Production task cards, budgets,
repo allowlists, and ledgers are runtime config supplied by `--config` or
`BB_PLANE_DIR`, not files tracked in this product repo. The checked-in planes
under `examples/` and `tests/fixtures/` are public fixtures only.

One Rust binary, two personalities:

```bash
bb serve                    # the plane: webhook ingress, cron, queue, dispatch
bb run <task>               # the same workflow as dispatch work, from a terminal
bb status --json            # operator truth: tasks, runs, queue, parked, DLQ
bb runs list --json         # durable ledger: state, agent@version, cost, duration
bb runs export              # versioned JSONL telemetry for evaluation/OTel adapters
bb dlq replay <id>          # dead letters replay as new runs with lineage
bb notify retry --json      # retry durable notification outbox rows
bb keys mint <agent>        # mint scoped OpenRouter child keys from policy caps
bb keys sync --all --check  # compare stored keys with provider caps/usage
bb task park|unpark <task>  # budget breaches park; unpark is explicit
bb recover                  # classify runs inherited from a dead plane
bb check                    # validate the config surface
```

## Agent skill

Agents that need to operate Bitterblossom can load the portable skill folder at
[`skills/bitterblossom/`](skills/bitterblossom/). Copy or symlink the whole
folder into a harness skill root; do not copy only `SKILL.md`, because the
recipes and UI metadata are part of the interface.

Use [`.agents/skills/bb-dogfood/`](.agents/skills/bb-dogfood/) when using `bb`
to deliver a PR from this repo's own backlog, capture primary-user friction,
bugs, delight, and synthesize follow-up backlog. This is repo-local because it
contains Bitterblossom's own dogfood loop rather than a portable `bb` operator
contract.

## Quick start

The one-minute zero-credential golden path — no secrets, no remote
substrate, no network. It validates the plane, dispatches a task, records
a run, exposes status, and reads the run bundle:

```bash
cargo build
./target/debug/bb --config examples/local-plane check --json
./target/debug/bb --config examples/local-plane preflight hello --json
./target/debug/bb --config examples/local-plane run hello --payload '{"ok":true}' --json
./target/debug/bb --config examples/local-plane status --json
./target/debug/bb --config examples/local-plane runs show <run-id> --json   # from the run --json output
./target/debug/bb --config examples/local-plane artifacts read <run-id> REPORT.json --json
```

`examples/local-plane/` is a zero-credential local plane: a `command`-harness
agent whose inline script writes a small `REPORT.json`. The `local` substrate
is dev/test machinery, rejected unless `plane.toml` sets `dev = true`.

`examples/demo-plane/` is a complete commented production-shaped config that
dispatches to a remote substrate: `sprites` restores checkpoints, syncs
repos, and executes the harness on a [Fly Sprite](https://sprites.dev) over
WebSocket exec.

```bash
./target/debug/bb --config examples/demo-plane check
./target/debug/bb --config examples/demo-plane run demo
```

Copyable workload templates beyond the demo live under `examples/`:

- `examples/review-factory-plane/` is a credential-free-to-validate
  pull-request review factory with agent policy, webhook containment filters,
  budgets, sample GitHub payload, expected `REPORT.json`, and a local
  validation recipe.
- `examples/canary-responder-plane/` is a credential-free-to-validate
  report-only incident responder for Canary-style wake-up events with
  containment filters, budgets, sample incident payload, expected
  `REPORT.json`, and a local validation recipe.
- `examples/docs-sync-plane/` is a credential-free-to-validate docs drift
  watcher with manual, cron, and GitHub push webhook triggers, containment
  filters, budgets, sample push payload, expected `REPORT.json`, and a local
  validation recipe.

Use a template when the trigger shape, output contract, and side-effect policy
already match the work: copy it into your runtime plane, change the org/repo/
host/secret names, tune budgets, then run `bb check`. Author a custom task when
the agent needs a different event schema, different allowed side effects, a
different report contract, or a workload-specific lane card. New workloads
should still be config only: plane/task/agent files plus lane cards and sample
payloads, not Rust changes.

```bash
./target/debug/bb --config examples/review-factory-plane check --json
./target/debug/bb --config examples/canary-responder-plane check --json
./target/debug/bb --config examples/docs-sync-plane check --json
```

## Guarantees

- A run row exists in SQLite **before any trigger gets its ack**; ingress
  is idempotent per trigger-defined dedupe key.
- Hosts never run two tasks at once (durable lease on substrate resource
  identity). Only pre-execute failures retry mechanically; everything
  after has side effects and is operator-resolved. Boot recovery probes
  the host instead of blindly orphaning inherited runs; `recover --json`
  separates `probe_state`, `probe_reason`, lease disposition, and the safe
  operator action.
- Budgets are tiered honestly: runs/day and the global daily ceiling are
  enforced pre-dispatch, streaming harness usage is metered in-flight against
  `max_cost_per_run_usd`, and a breach follows the agent side-effect policy
  (`kill` by default) through the notification outbox. Harnesses that only
  report final cost still park the task and notify after completion.
- Secrets are resolved per-exec and travel on stdin, never argv. API-auth
  agents with `policy.provider_key_name` use plane-side scoped OpenRouter child
  keys minted by `bb keys`; `bb keys sync --check` compares provider-side caps
  with agent policy. The management key is never injected into runs.
- Model & auth policy is code, not intent: claude/codex run on the
  operator's subscription auth only (`ANTHROPIC_API_KEY` /
  `OPENAI_API_KEY` are rejected as agent secrets), reflex triggers
  (webhook/cron) bind only `auth = "api"` agents — cheap open-weight
  models via OpenRouter on open harnesses (pi/omp). Api-auth execs are
  hermetic: scrubbed env, workspace-local HOME, declared secrets only.
- The plane holds no judgment: workloads are config, agents own their own
  decomposition, and a workload-specific branch in the spine is wrong by
  definition.

## Reading order

1. [VISION.md](VISION.md) — product boundary and north star
2. [project.md](project.md) — v3 direction lock record (2026-06-10)
3. [docs/spine.md](docs/spine.md) — the operator contract
4. [docs/adr/005-rust-event-plane.md](docs/adr/005-rust-event-plane.md) —
   why this shape (supersedes the Elixir conductor, ADR 003/004)
5. [docs/plans/2026-06-10-031-event-plane-spine.md](docs/plans/2026-06-10-031-event-plane-spine.md)
   — design + adversarial critique record

Prior incarnations (Python conductor, Go CLI, Elixir persona factory) live
in git history and `docs/archive/`; their surviving operational knowledge
is in harness-kit `skills/sprites/references/provisioning.md`.

## Build & test

```bash
cargo fmt --check && cargo clippy --all-targets -- -D warnings && cargo test
```
