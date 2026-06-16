# Simplification Model Evaluations

This directory records evaluator outputs for the simplification member of the
submission storm.

Current candidate set:

- `simplification` - `pi` / `deepseek/deepseek-v4-flash`
- `simplification-kimi` - `pi` / `moonshotai/kimi-k2.7-code`
- `simplification-glm` - `pi` / `z-ai/glm-5.1`

Current evaluator:

- `model-eval` - `openai/gpt-5.5`

Side-effect rule:

- Variant tasks use eval-only verdict kinds (`simplification-kimi`,
  `simplification-glm`). They parse verdict JSON and receive submission payload
  injection, but they are not canonical gate members.

Records:

- None yet. First record should compare the same submission and focus on whether
  each candidate finds real avoidable complexity without style nitpicks.
