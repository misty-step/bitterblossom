# CLAUDE.md

Claude-family tools read this file first. `AGENTS.md` symlinks here.

Also read:
- `project.md` — the v3 vision (2026-06-10 direction lock)
- `docs/spine.md` — the operator contract for `bb`
- `docs/adr/005-rust-event-plane.md` — why this shape
- `docs/plans/2026-06-10-031-event-plane-spine.md` — design + critique record

## What This Is

Bitterblossom is the **event plane** for agent workloads: define a task,
bind an agent, attach a trigger — all as files — and the plane runs it
durably on a Fly Sprite (or a local process) with cost, budget, and trace
visible from the CLI.

One Rust crate, one binary (`bb`), two personalities:

- `bb serve` — webhook ingress, cron scheduler, queue, dispatch.
- `bb <verb>` — operator/agent CLI on the same core (`run`, `runs`,
  `dlq`, `task`, `recover`, `check`). Every workflow also runs ad hoc
  from a terminal with no webhook.

Mode boundary (harness-kit `meta/CONTRACTS.md`): ad-hoc operator sessions
live in harness-kit (Mode A); recurring event-driven workflows live here
(Mode B). The plane holds **no judgment** — no workload logic, mechanical
retries only, agents own their own decomposition.

## Layout

```text
src/                 The spine (≤ ~5k LOC budget)
  spec.rs            Config loading: plane.toml, agents/, tasks/
  ledger.rs          SQLite run ledger, state machine, leases, dead letters
  ingress.rs         Webhook HMAC + dedupe, cron schedules
  dispatch.rs        Budget check → lease → prepare → execute → collect
  substrate/         local + sprites adapters (WorkspacePlan seam)
  harness.rs         claude/codex/pi command building + output parsing
  budget.rs          Enforced vs advisory tiers, named honestly
  recovery.rs        Boot classification of inherited runs (probe, no orphaning)
  serve.rs           tiny_http + cron + dispatch threads
  notify.rs          State-transition webhook (curl, best-effort)
tests/               e2e (local + stubbed sprites), ingress, recovery, budgets
examples/demo-plane/ Complete commented config surface (`bb check` validates)
backlog.d/           Work source; _done/ is the archive
docs/                Vision, ADRs, plans, spine contract, archive/
```

## Build & Test

```bash
cargo build                                 # binary: target/debug/bb
cargo fmt --check
cargo clippy --all-targets -- -D warnings
cargo test                                  # 37 tests, no network, no tokens
```

CI (`.github/workflows/ci.yml`) runs exactly those three gates.

## Gotchas (earned by pain)

- **Sprite exec transport is WebSocket** — never `--http-post` (cold
  sprites 502). The sprite CLI needs `--` before the remote command or it
  eats remote args as its own flags, and it resolves its org from the
  *cwd path history* — the adapter runs the relay from `$HOME`; use
  `org/name` host syntax to pin the org.
- **Secrets and prompts travel on stdin**, never argv: argv is visible in
  process tables and CLI telemetry. The heredoc delimiter is unguessable
  by design.
- **"Re-run it" is not a recovery semantic.** Agent runs have external
  side effects; only pre-execute failures retry mechanically. Everything
  at/after execute is operator-resolved (`bb runs resolve`).
- **A workload-specific branch in dispatch/queue/substrate is wrong by
  definition.** Workloads are task specs (config + lane card). The Python
  conductor (20k LOC) and the Elixir persona fleet both died of spine
  bloat — see `docs/archive/` and git history for the prior art.

## Coding Standards

- Rust stable, `cargo fmt`, clippy clean with `-D warnings`
- Deep modules (Ousterhout); code is a liability — every line fights for
  its life
- Semantic commits: `feat:`, `fix:`, `test:`, `docs:`, `refactor:`;
  backlog closure via `git interpret-trailers` (`Closes-backlog: <id>`)
