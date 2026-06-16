# Product Model Evaluations

This directory records evaluator outputs for the product member of the
submission storm.

Current candidate set:

- `product` - `pi` / `x-ai/grok-4.3`
- `product-kimi` - `pi` / `moonshotai/kimi-k2.7-code`
- `product-glm` - `pi` / `z-ai/glm-5.1`

Current evaluator:

- `model-eval` - `openai/gpt-5.5`

Side-effect rule:

- Variant tasks use eval-only verdict kinds (`product-kimi`, `product-glm`).
  They parse verdict JSON and receive submission payload injection, but they are
  not canonical gate members.

Records:

- None yet. First record should compare the same submission and focus on
  operator value, UX regressions, and product-scope judgement.
