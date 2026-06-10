# ADR-005: Rust Event Plane — Tasks, Agents, Triggers as Files

- **Status:** Accepted
- **Date:** 2026-06-10
- **Supersedes:** ADR-003 (conductor control plane), ADR-004 (Elixir conductor)
- **Related:** `docs/plans/2026-06-10-031-event-plane-spine.md` (context packet + critique record), `project.md` (v3 vision)

## Context

Bitterblossom v2 was an agent-first software factory: an Elixir conductor
provisioning a persona fleet (Weaver/Thorn/Fern/Tansy) running autonomous
loops. It worked, but the operating model inverted the wrong way — the
personas owned the workflow and the operator watched. Meanwhile the
portfolio boundary hardened (harness-kit `meta/CONTRACTS.md`): Mode A is
ad-hoc operator sessions on the personal machine; Mode B is event-driven
workflows that run somewhere durable, never by the authoring agent.

Olympus (adminifi) proved the Mode B shape in production: webhook
orchestrator, durable SQLite run ledger before ack, per-workflow queues,
dead letters, execution-substrate abstraction over Fly Sprites, versioned
agent specs, cost per job. Its limits are also instructive: TypeScript,
tenant-shaped, and every workload still needs TS dispatcher/preparer code.

Build-vs-borrow research (2026-06-10) found no off-the-shelf system that
dispatches a coding harness onto a remote sandbox: Temporal/Inngest/
Trigger.dev orchestrate in-process functions; Cloudflare's Agents SDK owns
its own substrate. The hard 20% would remain ours on any platform.

## Decision

Rewrite bitterblossom as a small Rust event plane — one crate, one binary
(`bb serve` + operator CLI) — implementing five primitives:

- **Task**: lane card (`card.md`) + `task.toml` (repos, substrate host,
  budgets, triggers, adapter commands). Files, not code.
- **Agent**: versioned harness+model binding (`agents/<name>.toml`),
  swappable with one config edit, recorded on every attempt.
- **Trigger**: webhook (HMAC), cron, manual — all converging on one
  ingress path: validate → trigger-defined dedupe key → durable run row →
  ack.
- **Run**: ledger row in SQLite (WAL) with attempt phase checkpoints
  (`acquired → … → released`), mechanical retries for pre-execute
  failures only, dead letters with replay lineage (`parent_run_id`), and
  boot-time classification of inherited runs via host probe — never blind
  orphaning, because agent runs have external side effects.
- **Substrate**: trait with `sprites` (shell out to the sprite CLI over
  WebSocket exec) and `local` (the degenerate case that keeps every task
  terminal-runnable) adapters. Host mutual exclusion is a durable lease
  keyed by substrate resource identity, not task.

Budgets are tiered honestly: runs/day and a global daily ceiling are
enforced pre-dispatch; the wall-clock timeout (substrate kill) is the v1
spend backstop; per-run cost is advisory — a breach parks the task
(`blocked_budget`) and notifies, bounding damage to one run.

The plane holds no judgment: no workload logic, no retry cleverness, no
opinion about what agents do. Workloads expressible as lane-card execution
over declared repos, secrets, triggers, and artifacts touch zero Rust.

## Deviations from the context packet

- **tiny_http + threads instead of axum + tokio.** The core is
  synchronous (process exec, SQLite) and ingress is two routes; an async
  runtime bought nothing but a bridging seam. Recorded here because the
  packet named axum.
- **Notifications shell out to curl** rather than carrying an HTTP client
  dependency; best-effort one-shot POST on state transitions only.

## Consequences

- The Elixir conductor (`conductor/`), persona fleet (`sprites/`), and
  factory docs are prior art pending teardown (backlog 032, unblocked once
  the sprites substrate has soaked).
- Workload #1 is the code review factory (backlog 028) as pure config on
  this spine; the Canary responder becomes a workload, not a persona.
- Spine LOC budget ≤ ~5k; a workload-specific branch in
  dispatch/queue/substrate is wrong by definition (the 20k-LOC Python
  conductor and the mothballed Elixir fleet are the two prior deaths this
  tripwire encodes).
