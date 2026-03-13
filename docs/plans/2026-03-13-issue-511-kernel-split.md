# Issue 511 Kernel Split Plan

> Scope issue: #511

## Goal

Split the conductor into explicit, testable modules without breaking the current CLI or run contract.

## Product Spec

### Problem

`scripts/conductor.py` is carrying tracker reads/writes, workspace lifecycle, GitHub polling, governance helpers, and orchestration in one file. That makes boundary changes risky and forces operators and agents to load too much unrelated context to reason about one part of the factory.

### Intent Contract

- Intent: move deep, cohesive responsibilities into named modules while keeping `scripts/conductor.py` as the stable entrypoint.
- Success Conditions: tracker, workspace, and governance behavior can be read and tested without scanning unrelated orchestration code.
- Hard Boundaries: no behavior rewrite, no prompt-only abstraction, no shallow pass-through modules that just rename old functions.
- Non-Goals: finish every target module from the epic in one slice or redesign the full workflow contract.

## Technical Design

### First Slice

1. Create `scripts/conductorlib/` as the conductor support package.
2. Extract shared models and command/runtime primitives needed by multiple modules.
3. Extract these real seams first:
   - `tracker.py`: GitHub issue/PR reads, comments, readiness helpers, QA issue sync helpers.
   - `workspace.py`: run workspace pathing plus prepare/cleanup/retry logic.
   - `governance.py`: PR check polling, trusted-surface settling, and review-thread helpers.
4. Keep `scripts/conductor.py` as the orchestrator and CLI facade, re-exporting moved functions so existing tests and callers stay stable during the split.
5. Add boundary-focused regression tests for the new modules instead of relying only on `scripts/test_conductor.py`.

### Follow-On Slices

- Move lease/run/event persistence into a state module.
- Split worker-slot scheduling and runner dispatch into separate modules.
- Isolate incident/waiver policy once the current trusted-surface logic is stable.

## Verification

- Focused pytest for each new module.
- Full `scripts/test_conductor.py`.
- Architecture docs updated to point at the new module layout.
