# Re-test the durable workflow build-vs-borrow decision after hardening

Priority: P2 | Status: pending | Estimate: M

## Goal

Re-evaluate whether Bitterblossom should keep its custom Rust scheduling loop,
adopt a durable workflow runtime for selected primitives, or document why the
current shape still wins.

## Oracle

- [ ] A decision memo compares the hardened `bb` loop against Temporal,
      Inngest, Trigger.dev, and Cloudflare-style worker/agent primitives.
- [ ] The comparison uses live stress evidence from 050/051, not speculation.
- [ ] The memo names primitives that are worth borrowing, primitives that must
      remain bespoke because they dispatch coding harnesses to sandboxes, and
      migration costs.
- [ ] The outcome is one of: keep custom spine with explicit proof, borrow a
      narrow primitive, or shape a migration epic.
- [ ] No runtime dependency is added during the decision pass.

## Children

1. Capture stress and recovery evidence after 050/051.
2. Compare required primitives: queues, retries, leases, recovery, visibility,
   cost tracking, sandbox dispatch, and file-first workload config.
3. Write the decision memo.
4. Emit follow-up only if the memo changes the architecture.

## Notes

Why: the premise challenger correctly forced this proof obligation. The current
direction can still be right, but only if the custom spine keeps earning its
small surface.

Evidence:

- `project.md:78-82` says prior research rejected Temporal/Inngest/Trigger.dev
  because none dispatch a coding harness onto a remote sandbox.
- External exemplar search found official durable workflow systems positioning
  retries, queues, observability, and state resume as first-class.
- 050/051 will produce the stress evidence needed for a fair re-check.
