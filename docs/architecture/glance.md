# docs/architecture glance

Purpose: visual, durable architecture artifacts for Bitterblossom.

## Files

- `README.md` — top-level system map, trace bullet, and navigation
- `conductor.md` — control-plane drill down: run state, governance, worker readiness
- `bb-cli.md` — transport drill down: setup, dispatch, status, logs

## Read Order

1. `README.md`
2. `conductor.md`
3. `bb-cli.md`

## Notes

- Diagrams are Mermaid-first so GitHub renders them natively.
- Keep these docs aligned with `project.md`, `docs/CONDUCTOR.md`, and ADR-003.
- Prefer updating the smallest affected doc instead of stuffing more detail into the overview.
