# Context Packet: Bitterblossom v3 Rust Event-Plane Spine

Deliverable: code — a Rust service + CLI (`bb`) implementing the v3
primitives well enough to carry workload #1 (backlog 028) end to end.
Consumed by `/deliver` against backlog 031.

## Goal

An operator defines a task, binds an agent, attaches a trigger — all as
files — and the plane runs it durably on a sprite with cost, budget, and
trace visible from the CLI.

## Non-Goals

- No workload logic in the plane (the review coordinator, incident
  judgment, etc. live in task specs and agent prompts — 028's problem).
- No dashboard/UI in v1; CLI with stable `--json` is the operator surface.
- No multi-tenancy, no auth beyond a shared ingress token + webhook HMAC.
- No LLM proxy or per-provider usage metering (Olympus `llm-proxy.ts`) in
  v1 — cost comes from harness output parsing; proxy is a later milestone.
- No Daedalus arena integration beyond emitting consumable run telemetry.
- No checkpoint *baking* — golden-checkpoint creation stays in harness-kit
  `/sprites` tooling; the plane only restores and verifies.

## Constraints (invariants)

- A run row exists in SQLite before any trigger gets its ack (Olympus's
  load-bearing lesson; their `workflow_runs` promoted from metrics store
  to ledger only after crash-recovery pain).
- Ingress is idempotent, and **each trigger spec defines its own dedupe
  key derivation and window** (webhook: header/body expression; cron: the
  scheduled timestamp; manual: explicit key, or an explicitly non-deduped
  run). `ingress_events` stores trigger id, source event id, and payload
  hash; semantic duplicates are recorded, never re-dispatched.
- Host mutual exclusion is keyed by **substrate resource identity, not
  task**: dispatch acquires a durable host lease before executing, so two
  tasks sharing a sprite can never overlap. Per-task FIFO ordering is a
  separate, weaker guarantee layered above the lease.
- Sprite credentials are per-exec and temporary; nothing persists on the
  sprite. Sprite exec transport is WebSocket, never `--http-post` (cold
  sprites 502 on HTTP POST — earned in this repo).
- Every Mode B flow runs ad hoc from a terminal with no webhook
  (harness-kit `meta/CONTRACTS.md`).
- Tasks/agents/triggers are data. Adding a workload **expressible as
  lane-card execution over declared repos, secrets, triggers, and
  artifacts** touches zero Rust. Source-specific payload normalization and
  side effects (e.g. posting a threaded PR reply, claiming a Canary
  incident) happen through declarative task adapter commands
  (`pre_command`/`post_command` run in the workspace), not plane code.
- The plane holds no judgment: it never decides what to fix, retry
  semantics are mechanical, agents own their own decomposition.
- Spine stays small — target ≤ ~5k LOC. The Python-conductor postmortem
  (20k LOC/7 days) is the anti-pattern; if a feature fights for a layer,
  it goes to the task spec or to harness-kit.

## Repo Anchors

Prior art to follow (read these before building):

- `adminifi/olympus/orchestrator/src/execution-substrate.ts` — the
  substrate trait: capabilities, acquire/ensure/execute/probe/cancel/
  read+write artifact/release. Port this shape to a Rust trait.
- `adminifi/olympus/orchestrator/src/db.ts:628-800` — proven ledger
  schema: `workflow_runs`, `run_ingress_events`, `jobs`, `pipeline_events`,
  `dead_letters`, `budget_events`. Adopt simplified.
- `adminifi/olympus/orchestrator/src/agent-specs.ts` — versioned agent
  spec (runtime, model, substrate ref, prompt ref, triggers, secrets refs,
  cost policy, trace identity). v3's agent file is this minus the
  per-lane TS `hooks` — the hooks are what config replaces.
- `adminifi/olympus/orchestrator/src/agent-runtimes.ts` — harness adapter:
  `buildCommand` per runtime (`codex exec` / `claude -p` / `pi -p`),
  output parsing for tokens/cost.
- `adminifi/olympus/orchestrator/src/job-queue.ts` — per-lane serialized
  drain with dispatch timeout that releases the drain but never silently
  drops the run.
- `harness-kit/skills/sprites/templates/lane-card.md` — the task-spec
  contract (shared with Mode A; the card is the agent's entire context).
- `harness-kit/skills/sprites/references/provisioning.md` — sprite
  lifecycle facts (golden checkpoints, wake semantics) the Elixir
  conductor learned the hard way.
- `docs/archive/` + `conductor/` (this repo) — prior art only; do not
  extend.

## Alternatives

**A. Bespoke Rust spine (recommended).** axum + rusqlite + tokio in one
binary. Fails if we over-build it — mitigated by the LOC budget and the
"workload = zero Rust" invariant forcing thinness.

**B. Fork/generalize Olympus.** Boring path; it's in production. Fails:
TypeScript (direction locked to Rust by the operator, 2026-06-10);
adminifi-shaped tenancy/repos baked through env and lane hooks; every
workload still requires TS dispatcher/preparer code — the exact coupling
v3 exists to remove. Verdict: steal its schema and seams, not its runtime.

**C. Durable-execution platform (Inngest/Trigger.dev/Temporal), utah-style.**
Fails: orchestrates in-process functions, so the hard 20% (sprite prepare,
harness exec, receipt collection) is still ours; adds a server/SaaS
dependency to a personal plane; SSPL/cloud coupling; non-Rust. Researched
2026-06-10 — nothing off-the-shelf dispatches a coding harness onto a
remote sandbox. Verdict: borrow `inngest/utah`'s trigger ergonomics, skip
the platform.

**D. No daemon: cron + CI + `dispatch-agent` scripts.** The truly boring
inversion — no resident process at all. Fails: no durable ledger or dedup
across triggers, no budget accounting, no dead-letter replay, and webhook
ingress needs a listening process anyway. Named because it sets the bar:
every spine feature must beat "a crontab and a shell script" on operator
value or it doesn't ship.

## Design

One Rust crate, one binary, two personalities: `bb serve` (the plane) and
`bb <verb>` (operator/agent CLI sharing the same core). Modules, not
microcrates: `ingress`, `ledger`, `queue`, `dispatch`, `substrate`,
`harness`, `spec` (task/agent/trigger loading), `budget`, `notify`, `cli`.
Root becomes a Cargo workspace when — not before — a second crate earns
existence.

**Config surface (the product):**

```
plane.toml                  # db path, ingress tokens, notify webhook, global budget
agents/<name>.toml          # versioned: harness, model, reasoning, skills/prompt ref
tasks/<name>/card.md        # lane card (harness-kit template) — the agent's context
tasks/<name>/task.toml      # repos, substrate host, budgets, agent binding, triggers
```

Trigger kinds: `webhook` (named route + HMAC/token), `cron` (in-process
tokio scheduler), `manual` (`bb run <task>`, the degenerate trigger). All
three converge on the same ingress fn: validate → idempotency check →
durable run row → ack → enqueue.

**Run state machine** (Olympus's, extended with attempt phases):
`pending → running → success | failure | awaiting_recovery`;
`kill_requested` flag; `blocked_budget` for parked-task ingress (event
recorded, no dispatch, reason visible in CLI; unpark is an explicit
operator command). Each attempt persists phase checkpoints —
`acquired → prepared → executing → collecting → finalizing → released` —
because agent runs have external side effects (pushed commits, posted
comments) and "re-run it" is not a recovery semantic. On boot, inherited
`running` runs are classified, not blindly orphaned: probe the host and
read artifacts/receipts for the existing attempt, then resolve to
`success`, `failure`, or `awaiting_recovery` (manual). Only attempts that
never reached `executing` are mechanically replayable. Retries: bounded
mechanical re-dispatch (2 after initial) **for pre-execute failures
only**; then dead-letter with full payload + attempt history. Replay
(`bb dlq replay <id>`) mints a new run with `parent_run_id` and a linked
trace, a fresh replay idempotency key, and an explicit operator choice of
original vs current task/agent spec versions.

**Substrate trait** (port of `ExecutionSubstrateAdapter`):
`capabilities()`, `acquire` (returns a durable host lease), `ensure`,
`prepare(WorkspacePlan)`, `execute`, `cancel`,
`read_artifact`/`write_artifact`, `release`. The substrate knows
environments and sessions, **not repos**: workspace materialization
(checkpoint restore, repo checkouts/hard-resets to declared refs, temp
credentials, preserved diagnostic paths) is a declarative `WorkspacePlan`
derived from the task spec and executed by one generic preparer, testable
identically on both adapters. Two adapters in v1:
`sprites` (shell out to the `sprite` CLI over WebSocket exec; HTTP API
adapter is a later swap behind the same trait) and `local` (process in a
temp worktree — this is what makes every task terminal-runnable and tests
cheap).

**Harness adapter:** `build_command(card, agent, budgets) -> Vec<String>`
plus `parse_output(stream) -> {result, turns, tokens, cost}` per runtime
id (`codex`, `claude`, `pi` at minimum). Harness ids and flags live in one
module; adding a harness is one adapter impl, not a schema change.

**Budgets — enforceable vs advisory, named honestly:** per-task
`{timeout_minutes, turn_cap, max_cost_per_run_usd, max_runs_per_day}` + a
global daily ceiling in `plane.toml`. *Enforced:* runs/day and ceiling
pre-dispatch; wall-clock timeout via substrate `cancel` — this is the v1
spend backstop. *Enforced only where the harness streams turn counts:*
turn cap (advisory otherwise). *Advisory in v1:* per-run cost — without an
LLM proxy, cost is known post-hoc from output parsing; a breach emits a
`budget_events` row + notification and parks the task (`blocked_budget`
semantics above) so the damage is bounded to one run. The packet says
this out loud so nobody mistakes parking for a hard cap.

**Ledger (SQLite, WAL):** `runs` (ledger + idempotency + trace_id),
`ingress_events` (every delivery incl. duplicates), `attempts` (per
dispatch: agent spec id+version, harness, model, timing, tokens, cost,
exit, artifact paths), `run_events` (milestones), `dead_letters`,
`budget_events`. Trace ID seeded at ingress, carried through retries and
receipts. Receipts also append to the harness-kit delegation-receipt JSONL
shape so Mode A tooling reads them.

**Observability:** `bb runs list|show --json` (agents are CLI users),
`GET /health` (queue depth, oldest pending, stalled drain), notification
webhook on state transitions only (dead-letter, budget breach, orphan at
boot, recovery) — workload #014's contract. Telemetry export for Daedalus
is a flat JSONL dump per run (`bb runs export`), OTel/Langfuse deferred.

**ADR decision:** record as ADR 005 (Rust event plane; supersedes ADR 004
Elixir conductor).

## Oracle

Phased; each phase is a commit-able milestone with its own gate.

1. **Spec + ledger + local substrate.** `bb run demo-task` (task defined
   only in config, agent = claude or codex) executes on the local adapter,
   writes `runs`/`attempts` rows with cost+duration; `bb runs show --json`
   returns them. `cargo test` covers state machine + idempotency
   (duplicate `bb run --idempotency-key X` creates one run, two ingress
   events).
2. **Sprites substrate.** Same demo task with `substrate = "sprites"`
   restores checkpoint, materializes the WorkspacePlan, executes, collects
   artifacts; run row identical in shape to local. Two tasks bound to the
   same sprite host never execute concurrently (host lease). Kill
   `bb serve` mid-run → on restart the run is classified via host
   probe/artifacts (not blindly orphaned), visible with its attempt phase,
   and only pre-execute attempts are auto-replayable.
3. **Ingress.** Webhook POST (valid HMAC) and a cron entry both produce
   durable runs through the same path; invalid signature is rejected with
   no row; a webhook redelivery with a different delivery id but the same
   trigger-defined dedupe key records an ingress event and no second run;
   a forced pre-execute dispatch failure dead-letters after bounded
   retries, and `bb dlq replay` mints a new run linked via
   `parent_run_id`.
4. **Budgets + swap.** Exceeding `max_runs_per_day` parks the task and
   fires the notification webhook; swapping the demo task's agent binding
   (codex → claude) is one config edit, and `bb runs list` shows which
   agent spec id+version produced which run.
5. CI gate (backlog 030) green on the Rust crate: fmt, clippy
   `-D warnings`, test.

## Critique Record

Adversarial fresh-context review by codex (gpt-5.5), artifact-only lane,
delegation receipt `e12ad318-4f9f-4065-959f-9693b1884137`
(`.harness-kit/traces/provider-lanes/20260610T173034.092493Z-codex-b72c8e0a.txt`).
3 blocking + 4 serious + 1 minor findings, all accepted and folded in:
attempt-phase recovery, trigger-defined dedupe, honest budget enforcement
tiers, WorkspacePlan seam, narrowed zero-Rust claim + task adapter
commands, host-lease mutual exclusion, replay lineage, parked semantics.

## Premise Source

sha256:e8c61e70a12cc5217cb4b8883646b510ad62010bbfb0abdeba82c8ecf9f8c380 project.md

## Risks + Rollout

- **Risk: spine bloat.** The conductor died of it twice (Python 20k, then
  Elixir mothballed). Tripwire: any PR adding a workload-specific branch
  to dispatch/queue/substrate is wrong by definition; LOC budget reviewed
  at each milestone.
- **Risk: sprite CLI drift.** Transport is behind the substrate trait;
  provisioning facts live in harness-kit's reference, refresh before M2.
- **Risk: cost parsing per harness is fragile.** Mitigate: treat
  unparseable output as `failure` with raw output preserved on the attempt
  row (Olympus does this), never as silent zero-cost success.
- **Rollout:** v3 lives at repo root alongside `conductor/` until M2
  passes, then 032 (teardown) unblocks and removes the Elixir surface.
  Undo path: the spine is additive until 032 lands; reverting is deleting
  the crate.
