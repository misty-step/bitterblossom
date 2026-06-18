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

Records:

- None yet. First record should compare the same shaped packet across all three
  tasks and include the pushed branch or dry-run evidence for each candidate.
