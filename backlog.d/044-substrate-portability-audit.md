# Make substrate adapters fully own their specifics

Priority: P3 · Status: pending · Estimate: S

## Goal

Adding a third substrate (Cloudflare containers, plain SSH host, …) is
one adapter file plus a `for_task` arm — no edits to dispatch, spec, or
ledger — because nothing sprite-specific leaks outside `src/substrate/`.

## Oracle

- [ ] The hardcoded `/home/sprite/bb/<task>` remote workspace path in
      dispatch.rs moves behind the adapter (the substrate decides where
      workspaces live; dispatch supplies only the task name)
- [ ] `WorkspacePlan` field docs are substrate-neutral: `checkpoint`
      means "snapshot to restore, adapters that lack snapshots ignore
      it", not "sprites only"; same pass over spec validation messages
- [ ] `grep -rn sprite src/ --include='*.rs'` outside `src/substrate/`
      returns only generic test fixtures/host names, no semantics
- [ ] docs/spine.md states the substrate contract: what an adapter must
      implement, and that substrate choice is per-task config

## Notes

**Why:** operator direction (2026-06-11) — Fly Sprites is the substrate
out of convenience, not commitment; sprite-specific design decisions are
suspect by default. The `Substrate`/`Session` seam already exists and is
mostly clean; this is a leak-plugging pass, not a redesign. Do **not**
build a Cloudflare adapter speculatively — this ticket only ensures that
when one is wanted, it's an adapter, not surgery.
