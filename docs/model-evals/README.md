# Model Evaluation Reference Context

Bitterblossom does not assume one model is best for a flow. Agentic flows should
run at least three materially different candidate configs, then a stronger
evaluator should compare their outputs and produce durable reference context for
future runs.

Current first-class loop:

1. Run three candidate tasks for the same objective and payload.
2. Pass their run ids, costs, and `REPORT.json` contents to `bb --config plane
   run model-eval --payload ... --json`.
3. Save the evaluator result under this directory as reference context.
4. Promote a new default only after the result is backed by receipts and the
   repo gate still passes.

Current first-class cohorts:

| Flow | Candidates | Notes |
|---|---|---|
| [`build`](build/README.md) | `build`, `build-kimi`, `build-glm` | Variants default to dry-run unless explicitly live. |
| [`review`](review/README.md) | `review`, `review-deepseek`, `review-glm` | Variants force measurement mode and never post PR comments. |
| [`gardener`](gardener/README.md) | `gardener`, `gardener-kimi`, `gardener-glm` | Variants force dry-run and never file duplicate ticket PRs. |
| [`ci-diagnose`](ci-diagnose/README.md) | `ci-diagnose`, `ci-diagnose-kimi`, `ci-diagnose-glm` | Reflex default plus manual variants. |
| [`correctness`](correctness/README.md) | `correctness`, `correctness-kimi`, `correctness-glm` | Variants use eval-only verdict kinds. |
| [`security`](security/README.md) | `security`, `security-kimi`, `security-glm` | Variants use eval-only verdict kinds. |
| [`simplification`](simplification/README.md) | `simplification`, `simplification-kimi`, `simplification-glm` | Variants use eval-only verdict kinds. |
| [`product`](product/README.md) | `product`, `product-kimi`, `product-glm` | Variants use eval-only verdict kinds. |

`z-ai/glm-5.2` is not a runnable OpenRouter API model in the API catalog as
checked on June 16, 2026. Swap GLM-family candidates from `z-ai/glm-5.1` only
after the catalog exposes a concrete id and a local `bb` dogfood run succeeds.

Evaluator:

| Task | Model | Role |
|---|---|---|
| `model-eval` | `openai/gpt-5.5` | Compare candidate reports and write reference context |

Reference notes should include exact `bb` run ids, model ids, cost, latency,
what was evaluated, evaluator report path, accepted conclusion, and residual
risk. Do not record a model as best for a flow after only a malformed run,
failed harness parse, or same-context self-review.

Variant tasks are candidate lanes, not automatic production fanout. Base tasks
keep webhook, cron, and canonical gate authority; variants are manual-only
unless the repo deliberately promotes a new default.
