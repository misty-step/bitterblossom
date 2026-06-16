# Model Evaluation Reference Context

Bitterblossom does not assume one model is best for a flow. Important flows
should run at least three materially different candidate configs, then a
stronger evaluator should compare their outputs and produce durable reference
context for future runs.

Current first-class loop:

1. Run three candidate tasks for the same objective and payload.
2. Pass their run ids, costs, and `REPORT.json` contents to `bb --config plane
   run model-eval --payload ... --json`.
3. Save the evaluator result under this directory as reference context.
4. Promote a new default only after the result is backed by receipts and the
   repo gate still passes.

The first implemented cohort is [`ci-diagnose`](ci-diagnose/README.md):

| Candidate task | Model | Family |
|---|---|---|
| `ci-diagnose` | `deepseek/deepseek-v4-flash` | DeepSeek |
| `ci-diagnose-kimi` | `moonshotai/kimi-k2.7-code` | Kimi |
| `ci-diagnose-glm` | `z-ai/glm-5.1` | GLM |

`z-ai/glm-5.2` is not a runnable OpenRouter API model on June 15, 2026. The
OpenRouter model page says the GLM 5.2 API releases on June 16, 2026; swap the
GLM-family candidate from `z-ai/glm-5.1` after the API catalog exposes it and a
local `bb` dogfood run succeeds.

Evaluator:

| Task | Model | Role |
|---|---|---|
| `model-eval` | `openai/gpt-5.5` | Compare candidate reports and write reference context |

Reference notes should include exact `bb` run ids, model ids, cost, latency,
what was evaluated, evaluator report path, accepted conclusion, and residual
risk. Do not record a model as best for a flow after only a malformed run,
failed harness parse, or same-context self-review.
