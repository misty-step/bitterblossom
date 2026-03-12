# Reviewer Evidence: Issue 481 Telemetry Ledger

## Merge Claim

The conductor now persists per-run telemetry samples from builder and reviewer artifacts, exposes the rolled-up fields on `show-runs` / `show-run`, and publishes a stable `show-metrics` read model for dashboards and operator sidecars.

## Walkthrough Script

1. Start from the regression gate: `scripts/test_conductor.py` must stay green because the telemetry storage path touches the run ledger and operator inspection surfaces.
2. Seed one real SQLite conductor database on this branch with a merged run plus builder/reviewer usage payloads.
3. Show `show-run` returning the new per-run telemetry contract:
   - `picked_at`, `completed_at`, `duration_seconds`, `outcome`, `turn_count`
   - aggregate token totals and `estimated_cost_usd`
   - `model_usage`, `provider_usage`, and raw `telemetry_samples`
4. Show `show-metrics` returning the aggregate read model:
   - `summary`
   - `recent_runs`
   - `timeline`
5. Tie the story back to the persistent regression guard that now protects the contract.

## Evidence

- Terminal transcript: [terminal-proof.txt](./terminal-proof.txt)
- Persistent verification: `python3 -m pytest -q scripts/test_conductor.py`

## Before / After

Before this branch, run inspection exposed status and blocking truth but left usage, cost, composition, and trend data trapped in raw artifacts or absent entirely.

After this branch, the conductor ledger stores telemetry directly and serves it through stable JSON read models that a dashboard or operator script can consume without opening SQLite tables or scraping provider logs.

## Residual Gap

This branch records telemetry only when builder/reviewer artifacts include usage metadata. It does not synthesize costs for legacy artifacts that omit model, provider, or token fields.
