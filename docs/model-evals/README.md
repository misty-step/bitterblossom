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

Configured OpenRouter model ids for Pi and OMP agents are also checked against
`tests/fixtures/openrouter-models-current.json` by `./scripts/verify.sh`.
The fixture is deterministic local gate input, not proof that a new model
should be promoted. Refresh it only from the live OpenRouter catalog and promote
agent defaults only after a flow-specific `bb` smoke plus a model-eval record.

| Model id | Current role |
|---|---|
| `deepseek/deepseek-v4-flash` | Gardener, CI diagnose, simplification |
| `deepseek/deepseek-v4-pro` | Review, correctness, security |
| `moonshotai/kimi-k2-thinking` | Storm arbiter |
| `moonshotai/kimi-k2.6:minimal` | Review default; catalog id `moonshotai/kimi-k2.6` |
| `moonshotai/kimi-k2.7-code` | Build comparison, gardener, CI diagnose, storm variants |
| `openai/gpt-5.5` | Model evaluator |
| `x-ai/grok-4.3` | Product review |
| `z-ai/glm-5.2` | Build default through OMP; GLM candidate variants |

`z-ai/glm-5.2` is a runnable OpenRouter API model as checked on June 16, 2026:
1M context at `$1.40 / $4.40` per 1M input/output tokens. GLM-family candidate
tasks now use `z-ai/glm-5.2`; keep historical records on their original model
ids when those receipts used GLM 5.1.

Adoption smokes:

- `ci-diagnose-glm` run `51f3f03980a6` completed successfully on
  `z-ai/glm-5.2` through Pi/OpenRouter on June 16, 2026, cost `$0.03326702`,
  duration 62.7s.
- Local OMP/GLM sentinel on June 18, 2026 returned `BB_OMP_GLM_SMOKE_OK` with
  cost `$0.0165166`; OMP does not consume stdin in print mode, so the `bb`
  harness command tells it to read `LANE_CARD.md` from the prepared workspace.

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
