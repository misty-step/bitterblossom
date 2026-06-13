# Export run telemetry into Daedalus and standard observability shapes

Priority: P1 | Status: pending | Estimate: L

## Goal

Make run outcomes, costs, tokens, failures, and agent versions exportable as a
stable telemetry contract that Daedalus and standard GenAI observability tools
can consume.

## Oracle

- [ ] A documented export schema maps runs and attempts to task, agent version,
      trigger, state, duration, cost, token counts, retry/DLQ status, and
      artifact pointers.
- [ ] The schema has fixtures and backwards-compatibility expectations.
- [ ] A Daedalus import or handoff contract is documented with example data.
- [ ] An OpenTelemetry GenAI span/metric mapping is documented for future
      adapters without requiring a sidecar in the default spine.
- [ ] `bb runs export` remains the integration seam or is replaced with a
      better explicitly versioned command.

## Children

1. Inventory current `bb runs export` fields and missing telemetry fields.
2. Define the versioned export contract.
3. Add fixtures and schema tests.
4. Write the Daedalus handoff document.
5. Document OTel GenAI mapping and what stays out of the Rust spine.

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
