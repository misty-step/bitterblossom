# Project: Bitterblossom

Canonical north star: `VISION.md`. This file records the 2026-06-10 v3
direction lock and historical pivot into the Rust event-plane shape.

## Direction Lock

**Current direction lock (2026-06-10): reimagined as the event plane.**
Bitterblossom is the Mode B runtime in the portfolio boundary
(harness-kit `meta/CONTRACTS.md`): an opinionated, thin control plane for
**event-driven agent workloads** — Olympus's shape, generalized beyond one
company's repos, rewritten in Rust. The Elixir conductor, the persona fleet
(Weaver/Thorn/Fern/Muse/Tansy as resident sprites), and the autonomous
backlog factory are prior art, not the product. Do not extend them.

## Vision

There are two ways to work with agents:

1. **Ad hoc** — an operator on a machine spins up a session, talks to it,
   work gets done. That is harness-kit's job (Mode A).
2. **Event-driven** — a trigger fires (cron, webhook, CI event) and a
   bespoke agent workflow executes on isolated infrastructure, unattended.
   That is bitterblossom's job (Mode B).

Bitterblossom is optimized for the second and usable for the first: every
workload is also runnable from a terminal, and local ad-hoc agents can use
the same surface to spin up sprites for heavy lifting they don't want to do
locally.

**North Star:** Define a task, bind an agent to it, attach a trigger — and
then watch it: durable run ledger, budgets, traces, receipts. Swap the
agent without touching the task. Change the task without touching the
plane. Evaluated agent contracts drop in as bindings, and run telemetry
feeds back to external labs or operator tooling for iteration.

**Target user:** The operator (and their ad-hoc agents) running a portfolio
of repos with a handful of recurring agent workloads — code review,
incident response, docs sync, scheduled probes.

**Key differentiators:** Tasks/agents/triggers are *data, not code* — a new
workload should not require writing runtime code. Local-first triggers
(webhook is one ingress among several). Budgets and cost per run are
first-class. Substrate-abstracted (Fly Sprites first, local exec as the
degenerate substrate).

## Primitives

| Primitive | What it is |
|---|---|
| **Task** | A workload spec, lane-card-shaped (goal, oracle, boundaries, repos, budget). Versioned in git. |
| **Agent** | A binding of harness + model + prompt/skills or a launch contract. Swappable independently of the task. |
| **Trigger** | cron, webhook, or manual CLI invocation. Many triggers may point at one task. |
| **Run** | One accepted unit of work: durable ledger row before ack, trace ID surviving retries, status machine, cost, receipts, artifacts, dead-letter on exhaustion. |
| **Substrate** | Where the run executes. Fly Sprites (checkpoint restore + repo sync) first; local process as fallback. One contract, no host-specific branches in workloads. |

## What we imitate from Olympus (proven in production)

- Durable run ledger (SQLite) written before the webhook returns `202`.
- Per-workload serialization queues; retry wrapper; dead-letter visibility
  and operator replay.
- Sprite checkpoint restore + hard-reset repo prep before every agent exec;
  temporary per-exec credentials, nothing persisted on disk.
- An execution-substrate boundary so lanes never construct host clients.
- Operator CLI with stable `--json` (agents are CLI users too), plus
  metrics routes and a dashboard for humans.
- Cost, tokens, and timing recorded per job; trace ID seeded at ingress.
- Scheduled (cron) work flowing through the same ledger/queue/retry shell
  as webhook work.

## What we do differently

- **Generic, not adminifi-shaped.** Olympus lanes are TypeScript code; here
  a workload is a task file + agent binding + trigger row.
- **Evaluated-agent integration.** Agents can arrive as launch contracts; run
  outcomes (cost, scores, failures) export back to external labs or operator
  tooling. Langfuse / OTel-shaped telemetry from day one.
- **Rust.** The spine is a small Rust service (ingress + ledger + queue +
  dispatch + CLI). Olympus and the 1.6K-LOC Elixir conductor both prove the
  spine is small; the moat is the contracts, not the framework. Build-vs-
  borrow was researched (2026-06-10): Temporal/Inngest/Trigger.dev
  orchestrate in-process functions and add a server/SaaS dependency;
  Cloudflare's SDK owns its own substrate. None of them dispatch a coding
  harness onto a remote sandbox — that part is ours either way. Closest
  prior art to borrow from: `inngest/utah`, Olympus `orchestrator/`.
- **Notification-first observability.** The operator gets pinged; the
  dashboard is drill-down, not a pane of glass to watch.

## Shared contracts

Harness-kit defines, bitterblossom consumes: `backlog.d/` + closure
trailers, lane cards, delegation receipts, evidence paths
(harness-kit `meta/CONTRACTS.md`). Every workload must be runnable ad hoc
from a terminal with no webhook.

## Workload roadmap

1. **Code review reflex:** a task and agent binding around whichever external
   review runner is current, with budget gates, one review per head SHA, and
   receipt capture.
2. **Incident responder:** incident webhook → investigate → repair → verify,
   expressed as a workload spec rather than a resident persona.
3. Monitor/deploy watchers and the unattended outer loop (the retired
   `/flywheel`), per the Mode B roadmap in CONTRACTS.md.

## Quality bar

- [ ] Accepted work is durable before the trigger gets its ack
- [ ] A new workload requires zero runtime-code changes
- [ ] An agent can be swapped on a task with one config change, and the
      run ledger shows which agent produced which outcome
- [ ] Cost, budget burn, retries, dead letters, and queue pressure are
      visible from the CLI without log spelunking
- [ ] Every workload runs from a terminal with no webhook
- [ ] Run telemetry exports in a shape external evaluators can consume

_Last updated: 2026-06-10, during the v3 reimagining session._
