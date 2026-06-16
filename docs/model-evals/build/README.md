# Build Model Evaluations

This directory records evaluator outputs for the `build` dispatch flow. Use it
when comparing which builder should implement shaped Bitterblossom slices.

Current candidate set:

- `build` - `codex` / `gpt-5.5` subscription auth
- `build-kimi` - `pi` / `moonshotai/kimi-k2.7-code`
- `build-glm` - `pi` / `z-ai/glm-5.2`

Current evaluator:

- `model-eval` - `openai/gpt-5.5`

Side-effect rule:

- `build-kimi` and `build-glm` default to dry-run when `dry_run` is absent.
  Set `"dry_run": false` and a unique `branch_slug` only when intentionally
  comparing live branch-producing builders.

Records:

- None yet. First record should compare the same shaped packet across all three
  tasks and include the pushed branch or dry-run evidence for each candidate.
