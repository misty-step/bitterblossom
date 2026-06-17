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
  `dlq`, `task`, `recover`, `check`). Every workflow also runs as
  dispatch work from a terminal with no webhook. Reflex work =
  trigger-fired (webhook/cron), api-auth agents on cheap OpenRouter
  models, hermetic. Dispatch work = operator-initiated, may use
  claude/codex on subscription auth. Never Anthropic/OpenAI API keys.

Mode boundary (harness-kit `meta/CONTRACTS.md`): ad-hoc operator sessions
live in harness-kit (Mode A); recurring event-driven workflows live here
(Mode B). The plane holds **no judgment** — no workload logic, mechanical
retries only, agents own their own decomposition.

## Layout

```text
src/                 The spine (≤5300 LOC; mechanism only — see Gotchas)
  spec.rs            Config loading: plane.toml, agents/, tasks/
  ledger.rs          SQLite run ledger, state machine, leases, dead letters
  ingress.rs         Webhook HMAC + dedupe, cron schedules
  dispatch.rs        Budget check → lease → prepare → execute → collect
  substrate/         sprites adapter + dev/test local exec (WorkspacePlan seam)
  harness.rs         claude/codex/pi command building + output parsing
  budget.rs          Enforced vs advisory tiers, named honestly
  recovery.rs        Boot classification of inherited runs (probe, no orphaning)
  serve.rs           tiny_http + cron + dispatch threads
  notify.rs          State-transition webhook (curl, best-effort)
tests/               e2e (local + stubbed sprites), ingress, recovery, budgets
examples/demo-plane/ Complete commented config surface (`bb check` validates)
backlog.d/           Work source; _done/ is the archive
docs/                Vision, ADRs, plans, spine contract, archive/
skills/              Product-owned exportable agent interface for `bb`
```

## Verification

The repo gate is one entrypoint, identical locally and in CI
(`.github/workflows/ci.yml`):

```bash
./scripts/verify.sh    # fmt, clippy, tests, bb check on both planes, LOC budget
```

A green gate is necessary, never sufficient. Changes to dispatch,
substrate, harness, or workload config also need **live evidence** —
exact recipes, all repeatable:

- **Reflex/dispatch run, end to end**: `export GH_TOKEN=$(gh auth token)`
  (plus `OPENROUTER_API_KEY` in env), then
  `bb --config plane run review --payload '{"repo":"o/r","pr":N}'`.
  Evidence = ledger row (`bb runs show <id>`: state, cost, tokens) AND
  the externally visible effect (the PR comment).
- **Containment storm drill**: dev plane + stub harness, N rapid signed
  webhook deliveries vs a small `max_runs_per_day`; assert FIFO runs,
  `blocked_budget` rows, parked task, `budget_blocked` notifications.
  (Worked example: 2026-06-10 drill — 5 events, 3 ran, 2 blocked.)
- **Read API / HTML QA**: `bb serve` + curl every `/api/*` route and `/`
  with and without `BB_API_TOKEN`.
  Repeatable local control-loop drill:
  `./scripts/control-loop-drill.sh`.
- **Submission-loop drill** (dev plane + stub harnesses, minutes,
  repeatable): seeded-flaw change → `bb submit open` → storm members via
  `bb run <kind> --idempotency-key "storm:<sub>:<kind>" --payload
  '{"submission":"<id>",...}'` → `bb gate` blocked naming the plantings →
  fix → round 2 `clear`. Then the termination drill (blockers persist →
  `escalated` exactly at max_rounds, one notify; dead-lettered required
  member → `escalated`, never eternal pending) and the arbiter drill
  (reject a blocking fingerprint → still blocks → arbiter `pass` naming
  it → rejected). Worked example: 2026-06-11 dev-plane drill. Live storm
  runs use the same recipe against `plane/` (sprites, real models);
  evidence = gate JSON both rounds + per-member costs.

Feedback loop after shipping a workload change: run one live review,
then check `/api/runs` (or `bb runs list --json`) for cost and outcome —
the ledger is the system of record, OpenRouter usage accounting feeds it
per attempt.

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
- **The `src/` LOC cap (`scripts/verify.sh`) is a proxy, not the goal.** The
  real invariant is *mechanism, not workload judgment*: config, ledger,
  dispatch, ingress, CLI, recovery belong in `src/`; anything that encodes what
  a workload decides belongs in `tasks/` + lane cards. When you hit the cap,
  first ask "is what I'm adding mechanism?" — if not, move it out (that shrinks
  the spine). Raise the cap only as a conscious re-baseline when `src/` is
  verifiably lean (no dead code, deep modules), never to sneak a change past
  the gate.

## Coding Standards

- Rust stable, `cargo fmt`, clippy clean with `-D warnings`
- Deep modules (Ousterhout); code is a liability — every line fights for
  its life
- Semantic commits: `feat:`, `fix:`, `test:`, `docs:`, `refactor:`;
  backlog closure via `git interpret-trailers` (`Closes-backlog: <id>`)
