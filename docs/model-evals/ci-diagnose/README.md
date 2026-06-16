# CI Diagnose Model Evaluations

This directory records evaluator outputs for the `ci-diagnose` flow. Use it as
reference context when choosing which diagnoser task to run next or when
deciding whether to promote a new default model.

Current candidate set:

- `ci-diagnose` — `deepseek/deepseek-v4-flash`
- `ci-diagnose-kimi` — `moonshotai/kimi-k2.7-code`
- `ci-diagnose-glm` — `z-ai/glm-5.1`

Current evaluator:

- `model-eval` — `openai/gpt-5.5`

Records:

- [`2026-06-15-no-failure-probe.md`](2026-06-15-no-failure-probe.md) —
  DeepSeek Flash won the no-failure CI probe; use a real failed run next before
  changing root-cause defaults.
