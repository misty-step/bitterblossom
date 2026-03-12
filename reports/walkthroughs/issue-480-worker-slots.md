# Issue 480 Walkthrough

## What Changed

`scripts/conductor.py` now persists builder worker capacity as logical slots in SQLite, drains slots after repeated readiness probe failures, records slot attribution on runs, and exposes `show-workers` for operator inspection.

## Verification

```bash
python3 -m pytest -q scripts/test_conductor.py
make lint-python
python3 scripts/conductor.py show-workers \
  --repo misty-step/bitterblossom \
  --worker noble-blue-serpent:2 \
  --worker moss \
  --desired-concurrency 2
```

## Evidence

- Regression coverage proves slot seeding, slot drain behavior, run-level slot attribution, and operator visibility in `scripts/test_conductor.py`.
- `show-workers` reports slot health, current assignment, backfill demand, and recent drain/selection actions from the same ledger used by the conductor.
