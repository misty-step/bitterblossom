# Adopt the Misty Step comic-ops aesthetic baseline

Priority: P2 · Status: done · Estimate: M

## Goal
Evaluate and adopt the noir-ledger comic-ops flavor for Bitterblossom dispatch,
run ledger, readiness, and receipt surfaces.

## Oracle
- [x] `DESIGN.md` or project docs name the chosen flavor, likely
      `noir-ledger`, plus any plane-specific component constraints.
- [x] A representative dispatch/readiness/receipt surface is rendered or mocked
      with ledgers, proof strips, caption bands, and hard square panels.
- [x] Aesthetic changes do not hide failure states, run costs, substrate
      readiness, or audit receipts.
- [x] The implementation uses `@misty-step/aesthetic` commit `9bbe0f9` or later,
      or records a deliberate no-adoption decision.
- [x] `./scripts/verify.sh` passes after implementation.

## Notes
Reference board:
`http://serenity.tail5f5eb4.ts.net:8788/bitterblossom-noir-ledger-concept.png`.

## Delivery Notes

Delivered 2026-07-02.

- Added `DESIGN.md` with the `noir-ledger` visual contract for BB operator
  surfaces: square panels, caption bands, proof strips, dense ledger grids, and
  no decorative gradients/glass/rounded cards.
- Added `docs/design-contract.md` with provenance and a deliberate
  `@misty-step/aesthetic` no-runtime-import decision for this Rust-only static
  HTML surface. If BB later gains a maintained JS UI build, adopt
  `@misty-step/aesthetic` at commit `9bbe0f9` or later instead of hand-copying
  tokens.
- Updated `src/operator.html` to mark `data-aesthetic="noir-ledger"`, render a
  live proof strip from `/api/status`, and use hard caption bands and square
  ledger panels while preserving cost, run, DLQ, trigger, lease, ingress, and
  artifact visibility.
- Added `tests/design_contract.rs` so the design contract and proof strip stay
  tied to the maintained operator surface.

Proof:

- `cargo test --locked --test design_contract --test serve -- --nocapture`
- `npx @google/design.md lint DESIGN.md`
- Disposable local render: `bb serve` on a copied public-plane fixture at
  `127.0.0.1:7099`; `curl /`, `/api/status`, and `/api/tasks`; Playwright
  screenshots at `/tmp/bb-074-dashboard-desktop.png` and
  `/tmp/bb-074-dashboard-mobile.png`.
- `./scripts/verify.sh`
