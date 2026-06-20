# Build Model Evaluations

This directory records evaluator outputs for the `build` dispatch flow. Use it
when comparing which builder should implement shaped Bitterblossom slices.

Current candidate set:

- `build` - `omp` / `z-ai/glm-5.2` API auth
- `build-kimi` - `pi` / `moonshotai/kimi-k2.7-code`
- `build-glm` - `pi` / `z-ai/glm-5.2` (same model, different harness surface)

Current evaluator:

- `model-eval` - `openai/gpt-5.5`

Side-effect rule:

- `build-kimi` and `build-glm` default to dry-run when `dry_run` is absent.
  Set `"dry_run": false` and a unique `branch_slug` only when intentionally
  comparing live branch-producing builders.

Default rationale:

- `build` moved from Codex subscription auth to OMP/GLM on June 18, 2026 after
  dogfood run `380ca26ed25b` failed before authoring because sprite-side Codex
  OAuth could not refresh. The promotion keeps authoring on the open API-auth
  path while retaining Kimi and Pi/GLM comparison lanes.
- On June 20, 2026, backlog 075 calibrated the default `build` cap to `$4.00`
  after a successful live OMP/GLM authoring run cost `$3.207397` and parked the
  lane at the stale `$2.00` cap. Same-packet dry-runs showed Pi/GLM is cheaper,
  but not yet production-default evidence because the candidate report noted
  local commits during `dry_run`.

Records:

- [2026-06-20 builder cost calibration](2026-06-20-builder-cost-calibration.md):
  same-packet dry-run comparison across `build`, `build-glm`, and `build-kimi`,
  plus `model-eval` run `a6d019b66cda`. Decision: keep `build` on OMP/GLM and
  raise `max_cost_per_run_usd` to `$4.00`; keep Pi/GLM as a promising
  comparator, not the default yet.
