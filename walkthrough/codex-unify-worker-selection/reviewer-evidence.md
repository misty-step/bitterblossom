# Reviewer Evidence

## Merge Claim

The conductor now chooses builders through one slot-claiming path, reaps stale terminal or orphaned slot owners before fresh selection, and governance adoption only claims a slot after the lease is confirmed, so default single-slot workers and explicit-capacity workers share the same readiness and reservation behavior without leaking or wedging slots on recovery paths.

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

- `scripts/conductor.py:1015`
- `scripts/conductor.py:1594`
- `scripts/conductor.py:3773`
- `scripts/conductor.py:4314`

### Real Execution

```text
$ python3 -m pytest -q base/hooks scripts/test_conductor.py
337 passed in 1.39s

$ python3 -m ruff check base/hooks scripts/conductor.py scripts/test_conductor.py
All checks passed!
```

### Targeted Regression Added

- `test_select_worker_slot_supports_default_single_slot_workers`
- `test_reap_terminal_worker_slots_clears_only_terminal_or_missing_assignments`
- `test_ensure_governance_run_does_not_claim_worker_slot_before_lease`
- `test_run_once_reclaims_stale_lease_and_reuses_terminal_worker_slot`
- `test_run_once_workspace_preparation_error_after_builder_handoff_does_not_record_false_failure`

These tests prove the slot selector handles the plain single-slot worker case directly, stale terminal or orphaned slot owners are reaped before fresh selection, governance adoption does not claim a worker slot before lease acquisition succeeds, reclaimed stale leases can reuse the freed slot, and post-handoff workspace-preparation failures still avoid false failure reporting on the current merge surface.

## Persistent Verification

- `python3 -m pytest -q base/hooks scripts/test_conductor.py`

## Residual Risk

This branch does not simplify the duplicated PR-thread governance branches inside `govern_pr_flow(...)`. That is still the next larger simplification target.
