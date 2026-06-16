# Security Model Evaluations

This directory records evaluator outputs for the security member of the
submission storm.

Current candidate set:

- `security` - `pi` / `deepseek/deepseek-v4-pro`
- `security-kimi` - `pi` / `moonshotai/kimi-k2.7-code`
- `security-glm` - `pi` / `z-ai/glm-5.2`

Current evaluator:

- `model-eval` - `openai/gpt-5.5`

Side-effect rule:

- Variant tasks use eval-only verdict kinds (`security-kimi`, `security-glm`).
  They parse verdict JSON and receive submission payload injection, but they are
  not canonical gate members.

Records:

- None yet. First record should compare the same submission and focus on
  reachable security findings, secret handling, and false-positive control.
