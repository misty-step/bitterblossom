# Reconcile the spine CLI contract with live bb help

Priority: P2 · Status: ready · Estimate: S

## Goal

Keep the operator contract in `docs/spine.md` aligned with the actual `bb`
CLI so agents do not execute stale commands.

## Oracle

- [ ] `docs/spine.md` documents `bb run <task> --payload '<json>'`, not the
      stale `--var` form.
- [ ] `docs/spine.md` documents `bb runs export` without the unsupported
      `--since` flag.
- [ ] A cheap regression check or focused test protects the exported skill and
      spine docs from drifting on these command examples.
- [ ] Verification includes `./target/debug/bb run --help`,
      `./target/debug/bb runs export --help`, and `./scripts/verify.sh`.

## Notes

Evidence from the 2026-06-13 groom:

- Live help exposes `bb run [OPTIONS] <TASK>` with `--payload <PAYLOAD>`.
- Live help exposes `bb runs export [OPTIONS]` with only `--config`.
- `docs/spine.md:356` still says `--var k=v`.
- `docs/spine.md:359` still says `runs export [--since ...]`.

## Mega groom disposition 2026-06-13

This is now part of the P0 contract work in
`backlog.d/050-event-plane-hardening-before-growth.md` and the broader docs
sweep in `backlog.d/057-current-contract-docs-and-noise-sweep.md`. Fixing the
two stale snippets is necessary but not sufficient; the durable outcome is a
cheap parity gate across live help, docs, and the exported skill recipes.
