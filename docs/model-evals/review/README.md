# Review Model Evaluations

This directory records evaluator outputs for the `review` PR-review flow. Use it
when choosing the default reviewer model for GitHub PR review comments.

Current candidate set:

- `review` - `pi` / `moonshotai/kimi-k2.6:minimal`
- `review-deepseek` - `pi` / `deepseek/deepseek-v4-pro`
- `review-glm` - `pi` / `z-ai/glm-5.2`

Current evaluator:

- `model-eval` - `openai/gpt-5.5`

Side-effect rule:

- `review-deepseek` and `review-glm` force measurement mode, so they produce
  JSON evidence but never post duplicate public PR comments.

Records:

- [`2026-06-16-pr-855-measurement.md`](2026-06-16-pr-855-measurement.md) -
  GLM 5.1 won the first clean-PR measurement probe before the GLM 5.2 config
  update; use a seeded-finding or known-regression PR before changing the
  production default.
