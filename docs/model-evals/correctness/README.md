# Correctness Model Evaluations

This directory records evaluator outputs for the correctness member of the
submission storm.

Current candidate set:

- `correctness` - `pi` / `deepseek/deepseek-v4-pro`
- `correctness-kimi` - `pi` / `moonshotai/kimi-k2.7-code`
- `correctness-glm` - `pi` / `z-ai/glm-5.2`

Current evaluator:

- `model-eval` - `openai/gpt-5.5`

Side-effect rule:

- Variant tasks use eval-only verdict kinds (`correctness-kimi`,
  `correctness-glm`). They parse verdict JSON and receive submission payload
  injection, but they are not canonical gate members.

Records:

- None yet. First record should compare the same submission and include the gate
  report plus each candidate verdict JSON.
