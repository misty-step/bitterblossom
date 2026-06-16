# Gardener Model Evaluations

This directory records evaluator outputs for the `gardener` ledger-mining flow.
Use it when comparing which model should turn plane history into falsifiable
backlog tickets.

Current candidate set:

- `gardener` - `pi` / `deepseek/deepseek-v4-flash`
- `gardener-kimi` - `pi` / `moonshotai/kimi-k2.7-code`
- `gardener-glm` - `pi` / `z-ai/glm-5.2`

Current evaluator:

- `model-eval` - `openai/gpt-5.5`

Side-effect rule:

- `gardener-kimi` and `gardener-glm` force dry-run and never file duplicate
  ticket PRs.

Records:

- None yet. First record should use the same API window and compare evidence
  quality, ticket specificity, and restraint.
