# CI Diagnose Model Evaluations

This directory records evaluator outputs for the `ci-diagnose` flow. Use it as
reference context when choosing which diagnoser task to run next or when
deciding whether to promote a new default model.

Current candidate set:

- `ci-diagnose` — `deepseek/deepseek-v4-flash`
- `ci-diagnose-kimi` — `moonshotai/kimi-k2.7-code`
- `ci-diagnose-glm` — `z-ai/glm-5.2`

GLM 5.2 adoption smoke: `ci-diagnose-glm` run `51f3f03980a6` completed
successfully against the real failed-run payload from run `24208282343`, cost
`$0.03326702`, duration 62.7s. The historical records below keep their original
GLM 5.1 ids where those runs predated the GLM 5.2 config update.

Current evaluator:

- `model-eval` — `openai/gpt-5.5`

Records:

- [`2026-06-16-real-failure-diagnosis.md`](2026-06-16-real-failure-diagnosis.md) —
  GLM 5.1 won the real-failed-run evaluation for CI root-cause diagnosis
  quality before the GLM 5.2 config update; use it as the strongest reference
  for failed-log diagnosis until a broader GLM 5.2 sample says otherwise.
- [`2026-06-15-no-failure-probe.md`](2026-06-15-no-failure-probe.md) —
  Kimi K2.7 Code won the no-failure CI probe; use a real failed run next before
  changing root-cause defaults.
