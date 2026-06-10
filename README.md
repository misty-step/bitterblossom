# Bitterblossom

The event plane for agent workloads. Define a **task**, bind an **agent**,
attach a **trigger** — all as files — and the plane runs it durably on a
Fly Sprite (or a local process) with cost, budget, and trace visible from
the CLI.

```
plane.toml                  # db path, ingress bind, notify webhook, global budget
agents/<name>.toml          # versioned agent binding: harness, model, secrets
tasks/<name>/card.md        # lane card — the agent's entire context
tasks/<name>/task.toml      # agent, substrate, workspace, budgets, triggers
```

One Rust binary, two personalities:

```bash
bb serve                    # the plane: webhook ingress, cron, queue, dispatch
bb run <task>               # the same workflow, ad hoc from a terminal
bb runs list --json         # durable ledger: state, agent@version, cost, duration
bb dlq replay <id>          # dead letters replay as new runs with lineage
bb task park|unpark <task>  # budget breaches park; unpark is explicit
bb recover                  # classify runs inherited from a dead plane
bb check                    # validate the config surface
```

## Quick start

```bash
cargo build
./target/debug/bb --config examples/demo-plane check
./target/debug/bb --config examples/demo-plane run demo
```

`examples/demo-plane/` is a complete commented config. The `local`
substrate keeps every task terminal-runnable and tests cheap; the
`sprites` substrate restores checkpoints, syncs repos, and executes the
harness on a [Fly Sprite](https://sprites.dev) over WebSocket exec.

## Guarantees

- A run row exists in SQLite **before any trigger gets its ack**; ingress
  is idempotent per trigger-defined dedupe key.
- Hosts never run two tasks at once (durable lease on substrate resource
  identity). Only pre-execute failures retry mechanically; everything
  after has side effects and is operator-resolved. Boot recovery probes
  the host instead of blindly orphaning inherited runs.
- Budgets are tiered honestly: runs/day and the global daily ceiling are
  enforced pre-dispatch, the wall-clock kill is the spend backstop, and
  per-run cost is advisory — a breach parks the task and notifies.
- Secrets are resolved per-exec from the plane's environment and travel
  on stdin, never argv, never persisted.
- The plane holds no judgment: workloads are config, agents own their own
  decomposition, and a workload-specific branch in the spine is wrong by
  definition.

## Reading order

1. [project.md](project.md) — vision and direction lock (2026-06-10)
2. [docs/spine.md](docs/spine.md) — the operator contract
3. [docs/adr/005-rust-event-plane.md](docs/adr/005-rust-event-plane.md) —
   why this shape (supersedes the Elixir conductor, ADR 003/004)
4. [docs/plans/2026-06-10-031-event-plane-spine.md](docs/plans/2026-06-10-031-event-plane-spine.md)
   — design + adversarial critique record

Prior incarnations (Python conductor, Go CLI, Elixir persona factory) live
in git history and `docs/archive/`; their surviving operational knowledge
is in harness-kit `skills/sprites/references/provisioning.md`.

## Build & test

```bash
cargo fmt --check && cargo clippy --all-targets -- -D warnings && cargo test
```
