# Reviewer Evidence

## Merge Claim

The conductor now chooses builders through one slot-claiming path, so default single-slot workers and explicit-capacity workers share the same readiness probe and reservation behavior.

## Why This Matters

Before this branch, `run_once` and governance adoption could follow different worker-selection paths:

- default workers used `select_worker(...)` and then claimed a slot later
- explicit-capacity workers used `select_worker_slot(...)` and claimed the slot during selection

That split meant callers had to remember two selection contracts, and the default path had a race between "picked worker" and "reserved worker slot."

## Observable After State

- `select_worker(...)` is removed from the active conductor path
- both `run_once(...)` and `ensure_governance_run(...)` now call `select_worker_slot(...)`
- default single-slot worker specs like `["thorn", "sage"]` are covered by the slot-selection tests

## Evidence

### Code Path

- `scripts/conductor.py:1578`
- `scripts/conductor.py:3678`
- `scripts/conductor.py:4291`

### Real Execution

```text
$ python3 -m pytest -q scripts/test_conductor.py
222 passed in 2.31s

$ python3 -m ruff check scripts/conductor.py scripts/test_conductor.py
All checks passed!
```

### Targeted Regression Added

- `scripts/test_conductor.py:2932`

This test proves the slot selector now handles the plain single-slot worker case directly.

## Persistent Verification

- `python3 -m pytest -q scripts/test_conductor.py`

## Residual Risk

This branch does not simplify the duplicated PR-thread governance branches inside `govern_pr_flow(...)`. That is still the next larger simplification target.
