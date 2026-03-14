# docs/architecture glance

Purpose: visual, durable architecture artifacts for Bitterblossom.

## Files

- [`README.md`](./README.md) — top-level system map, trace bullet, and navigation
- [`conductor.md`](./conductor.md) — control-plane drill down: run state, governance, worker readiness
- [`bb-cli.md`](./bb-cli.md) — transport drill down: setup, dispatch, status, logs, kill
- [`skills.md`](./skills.md) — skill inventory, provisioning, and WORKFLOW.md contract

## Read Order

1. [`README.md`](./README.md)
2. [`conductor.md`](./conductor.md)
3. [`bb-cli.md`](./bb-cli.md)
4. [`skills.md`](./skills.md)

## Notes

- Diagrams are Mermaid-first so GitHub renders them natively.
- Keep these docs aligned with [`project.md`](../../project.md), [`docs/CONDUCTOR.md`](../CONDUCTOR.md), and [ADR-004](../adr/004-elixir-conductor-architecture.md).
- Prefer updating the smallest affected doc instead of stuffing more detail into the overview.
