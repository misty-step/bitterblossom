# Reviewer Evidence

## Merge Claim

The conductor now chooses builders through one slot-claiming path, and governance adoption only claims that slot after the lease is confirmed, so default single-slot workers and explicit-capacity workers share the same readiness and reservation behavior without leaking slots on lease contention.

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
$ python3 -m pytest -q base/hooks scripts/test_conductor.py
335 passed in 1.37s

$ python3 -m ruff check base/hooks scripts/conductor.py scripts/test_conductor.py
All checks passed!
```

### Targeted Regression Added

- `scripts/test_conductor.py:2932`
- `scripts/test_conductor.py:4814`
- `scripts/test_conductor.py:6652`

These tests prove the slot selector handles the plain single-slot worker case directly, governance adoption does not claim a worker slot before lease acquisition succeeds, and post-handoff workspace-preparation failures still avoid false failure reporting on the current merge surface.

## Persistent Verification

- `python3 -m pytest -q base/hooks scripts/test_conductor.py`

## Residual Risk

This branch does not simplify the duplicated PR-thread governance branches inside `govern_pr_flow(...)`. That is still the next larger simplification target.
