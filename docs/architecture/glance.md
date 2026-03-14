# docs/architecture glance

Purpose: visual, durable architecture artifacts for Bitterblossom.

## Files

- [`README.md`](./README.md) — top-level system map, trace bullet, and navigation
- [`conductor.md`](./conductor.md) — Elixir/OTP conductor: supervision tree, run state machine, key interfaces
- [`bb-cli.md`](./bb-cli.md) — Go transport drill down: setup, dispatch, status, logs, kill
- [`skills.md`](./skills.md) — repo-local skills: layers, provisioning contract, phase mapping

## Read Order

1. [`README.md`](./README.md)
2. [`conductor.md`](./conductor.md)
3. [`bb-cli.md`](./bb-cli.md)
4. [`skills.md`](./skills.md)

## Notes

- Diagrams are Mermaid-first so GitHub renders them natively.
- Keep these docs aligned with [`CLAUDE.md`](../../CLAUDE.md) and [`WORKFLOW.md`](../../WORKFLOW.md).
- The conductor doc covers the Elixir/OTP rewrite (PR #612+). The old Python conductor docs are archived.
- Prefer updating the smallest affected doc instead of stuffing more detail into the overview.
