# Re-test durable workflow and substrate build-vs-borrow decisions

Priority: P2 | Status: done | Estimate: M

## Goal

Re-evaluate whether Bitterblossom should keep its custom Rust scheduling loop,
adopt a durable workflow runtime for selected primitives, swap or add a
substrate adapter, or document why the current Fly/Sprites-first shape still
wins.

## Oracle

- [x] A decision memo compares the hardened `bb` loop against Temporal,
      Inngest, Trigger.dev, and Cloudflare-style worker/agent primitives.
- [x] A substrate bakeoff compares Fly/Sprites, Cloudflare Sandbox/Agents, E2B,
      Modal, and Daytona against the same coding-harness workload contract.
- [x] The workload contract prepares a real repo workspace, streams harness
      stdout/stderr, supports stdin/secrets without leaking payloads, captures
      artifacts, records cost, handles timeout/kill, and proves either
      persistence/checkpoint recovery or an explicit no-persistence tradeoff.
- [x] The comparison uses live stress evidence from 050/051, not speculation.
- [x] The memo names primitives that are worth borrowing, primitives that must
      remain bespoke because they dispatch coding harnesses to sandboxes, and
      migration costs.
- [x] The outcome is one of: keep custom spine and Fly/Sprites baseline with
      explicit proof, borrow a narrow workflow primitive, add a second substrate
      adapter, replace the current substrate, or shape a migration epic.
- [x] No runtime dependency is added during the decision pass.

## Children

1. [x] Capture stress and recovery evidence after 050/051.
2. [x] Compare required primitives: queues, retries, leases, recovery, visibility,
   cost tracking, sandbox dispatch, and file-first workload config.
3. [x] Define one repeatable coding-harness workload and scoring rubric: cold start,
   max runtime, disk/workspace size, persistent state, streaming/interactive
   affordances, secret handling, artifact extraction, network egress,
   concurrency/cost, failure recovery, and operator CLI ergonomics.
4. [x] Prototype only the smallest adapter probes needed to score the candidates.
5. [x] Write the decision memo.
6. [x] Emit follow-up only if the memo changes the architecture. No follow-up
   emitted: the decision keeps the current architecture.

## Notes

Why: the premise challenger correctly forced this proof obligation. The current
direction can still be right, but only if the custom spine and current substrate
keep earning their small surface.

Evidence:

- `project.md:78-82` says prior research rejected Temporal/Inngest/Trigger.dev
  because none dispatch a coding harness onto a remote sandbox.
- External exemplar search found official durable workflow systems positioning
  retries, queues, observability, and state resume as first-class.
- Direction lock 2026-06-29: Bitterblossom supports both supervised dispatch and
  unsupervised reflex work; substrate choice is an adapter decision, not product
  identity.
- 050/051 will produce the stress evidence needed for a fair re-check.
- 2026-07-02 child 1 captured the post-050/051 evidence baseline in
  `docs/build-vs-borrow/2026-07-02-stress-recovery-evidence.md`.
- 2026-07-02 child 2 defined the required primitive baseline in
  `docs/build-vs-borrow/2026-07-02-required-primitives.md`.
- 2026-07-02 child 3 defined the repeatable coding-harness workload and
  scoring rubric in `docs/build-vs-borrow/2026-07-02-workload-rubric.md`.
- 2026-07-02 child 4 added `scripts/build-vs-borrow-local-probe.sh` and
  recorded the local baseline evidence in
  `docs/build-vs-borrow/2026-07-02-local-probe-evidence.md`.
- 2026-07-02 child 5 closed the decision memo in
  `docs/build-vs-borrow/2026-07-02-decision.md`: keep the custom Rust spine and
  Fly/Sprites baseline, add no runtime dependency, and emit no architecture
  follow-up.
