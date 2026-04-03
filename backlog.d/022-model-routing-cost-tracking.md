# Model routing and cost tracking

Priority: high
Status: ready
Estimate: L

## Goal
Support tiered model selection per sprite/phase and track token costs. The factory cannot afford frontier models for every task when using API keys.

## Context
Pro Plan OAuth provides subsidized tokens today, but API key usage will be needed for:
- Fleet scale-out beyond Pro Plan limits
- Cross-provider model diversity (OpenRouter, Anthropic direct)
- Harness diversity (Claude Code sprites alongside Codex sprites)

## Model tiers
- **Frontier**: GPT 5.4, Claude Opus 4.6 — architecture review, complex implementation
- **Workhorse**: Codex 5.3, GPT 5.4 Mini, Claude Sonnet 4.6 — routine build, fix, polish
- **Fast**: Spark, Haiku — context reading, triage, simple fixes
- **OpenRouter**: cross-provider access (Gemini, Mistral, etc.) for specific tasks

## Sequence
- [ ] Add `model` and `provider` fields to fleet.toml per-sprite config (already has `model`)
- [ ] Add `reasoning_effort` per-sprite (already exists)
- [ ] Add OpenRouter provider support to the harness dispatch
- [ ] Add token tracking to Store events (input_tokens, output_tokens, model, cost_usd)
- [ ] Add per-sprite budget limits (daily/weekly token caps)
- [ ] Dashboard: show token usage and cost per sprite, per run

## Oracle
- [ ] Different sprites can use different models and providers
- [ ] Token usage is tracked per-session in Store events
- [ ] Budget limits prevent runaway cost
