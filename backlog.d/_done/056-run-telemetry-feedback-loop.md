# Export run telemetry into Daedalus and standard observability shapes

Priority: P1 | Status: done | Estimate: L

## Goal

Make run outcomes, costs, tokens, failures, and agent versions exportable as a
stable telemetry contract that Daedalus and standard GenAI observability tools
can consume.

## Oracle

- [x] A documented export schema maps runs and attempts to task, agent version,
      trigger, state, duration, cost, token counts, retry/DLQ status, and
      artifact pointers.
- [x] The schema has fixtures and backwards-compatibility expectations.
- [x] A Daedalus import or handoff contract is documented with example data.
- [x] An OpenTelemetry GenAI span/metric mapping is documented for future
      adapters without requiring a sidecar in the default spine.
- [x] `bb runs export` remains the integration seam or is replaced with a
      better explicitly versioned command.

## Children

1. Inventory current `bb runs export` fields and missing telemetry fields.
2. Define the versioned export contract.
3. Add fixtures and schema tests.
4. Write the Daedalus handoff document.
5. Document OTel GenAI mapping and what stays out of the Rust spine.

## Delivery 2026-06-16

- `bb runs export` now emits `bb.run_telemetry.v1` JSONL instead of an
  incidental raw ledger dump.
- The v1 envelope includes normalized run, attempt, retry, DLQ, artifact,
  Daedalus handoff, and OTel GenAI mapping fields.
- `docs/run-telemetry-export-v1.md` documents schema fields, units,
  compatibility rules, Daedalus example data, and the OTel mapping boundary.
- `tests/fixtures/run-telemetry-v1.jsonl` is the compatibility fixture and
  `tests/run_export.rs` verifies both the fixture and live CLI output.
- The SQLite schema moved to `src/schema.sql` and remains bundled by
  `include_str!`, keeping the Rust spine below the 5k LOC cap.
- Dogfood notes were captured in
  `docs/plans/2026-06-16-run-telemetry-export-dogfood.md`.

Evidence:

- Red test first: `cargo test --test run_export -- --nocapture` failed on the
  old raw export because `schema` was missing.
- Focused tests after implementation: `cargo test --test run_export -- --nocapture`
  and `cargo test --test ledger -- --nocapture` passed.
- Repo gate: `./scripts/verify.sh` passed with `src LOC: 4987`.
- Live dogfood: `./target/debug/bb --config plane runs export | head -n 2`
  emitted real `bb.run_telemetry.v1` rows for `product` and `security`.
- Parser dogfood: exported rows parsed as
  `bb.run_telemetry.v1 product bitterblossom openrouter` and
  `bb.run_telemetry.v1 security`.

## Notes

Why: the project vision names Daedalus and OTel-shaped telemetry, but the
current export is not yet a product contract.

Evidence:

- `project.md:72-74` says run outcomes export back to Daedalus and mentions
  OTel-shaped telemetry.
- `project.md:111` requires run telemetry exports in a shape Daedalus can
  consume.
- `docs/spine.md:249-255` currently defers deeper traces to `bb runs export`.
- Current OpenTelemetry GenAI agent span and metric conventions are mature
  enough to use as an optional mapping target.
