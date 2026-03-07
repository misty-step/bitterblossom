# docs/architecture glance

Purpose: visual, durable architecture artifacts for Bitterblossom.

## Files

- [`README.md`](./README.md) — top-level system map, trace bullet, and navigation
- [`conductor.md`](./conductor.md) — control-plane drill down: run state, governance, worker readiness
- [`bb-cli.md`](./bb-cli.md) — transport drill down: setup, dispatch, status, logs, kill

## Read Order

1. [`README.md`](./README.md)
2. [`conductor.md`](./conductor.md)
3. [`bb-cli.md`](./bb-cli.md)

## Notes

- Diagrams are Mermaid-first so GitHub renders them natively.
- Keep these docs aligned with [`project.md`](../../project.md), [`docs/CONDUCTOR.md`](../CONDUCTOR.md), and [ADR-003](../adr/003-conductor-control-plane.md).
- Prefer updating the smallest affected doc instead of stuffing more detail into the overview.
