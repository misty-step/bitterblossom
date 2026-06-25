# Review Model Evaluations

This directory records evaluator outputs for prompt-based PR-review candidates.
Production `review` now wraps Cerberus; keep this flow for comparing direct
model lane cards before any future promotion into a specialist wrapper.

Current candidate set:

- `review-kimi` - `pi` / `moonshotai/kimi-k2.6:minimal`
- `review-deepseek` - `pi` / `deepseek/deepseek-v4-pro`
- `review-glm` - `pi` / `z-ai/glm-5.2`

Current evaluator:

- `model-eval` - `openai/gpt-5.5`

Side-effect rule:

- `review-kimi`, `review-deepseek`, and `review-glm` are manual-only
  measurement tasks, so they produce JSON evidence but never post duplicate
  public PR comments.

Records:

- [`2026-06-16-pr-855-measurement.md`](2026-06-16-pr-855-measurement.md) -
  GLM 5.1 won the first clean-PR measurement probe before the GLM 5.2 config
  update, when the Kimi candidate still used the task name `review`; use a
  seeded-finding or known-regression PR before changing the prompt-candidate
  set or production wrapper.
