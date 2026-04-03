# Model routing and cost tracking

Priority: high
Status: ready
Estimate: L

## Goal
Support tiered model selection per sprite/phase and track token costs. The factory cannot afford frontier models for every task when using API keys.

## Context
Pro Plan OAuth provides subsidized tokens today, but API key usage is needed for fleet scale-out, cross-provider diversity (OpenRouter, Anthropic), and harness diversity (Claude Code sprites alongside Codex sprites).

## Model tiers
- **Frontier**: GPT 5.4, Claude Opus 4.6 — architecture review, complex implementation
- **Workhorse**: Codex 5.3, GPT 5.4 Mini, Claude Sonnet 4.6 — routine build, fix, polish
- **Fast**: Spark, Haiku — context reading, triage, simple fixes
- **OpenRouter**: cross-provider access (Gemini, Mistral, etc.) for specific tasks

## Sequence
- [ ] Extend fleet.toml `[defaults]` and `[[sprite]]` to support `provider` field (values: `codex`, `openai`, `openrouter`, `anthropic`)
- [ ] Add `Conductor.Codex.dispatch_command/1` support for model override via fleet.toml (already partially exists via `model` field)
- [ ] Create `Conductor.TokenTracker` module: receives token usage events, stores to SQLite via Store
- [ ] Add token tracking fields to Store events: `input_tokens`, `output_tokens`, `model`, `estimated_cost_usd`
- [ ] Parse Codex session output for token usage (Codex JSON events include token counts)
- [ ] Add per-sprite daily budget limit to fleet.toml: `daily_token_budget = 500000`
- [ ] When budget exceeded: pause sprite, record event, notify (ties into 014-notification)
- [ ] Dashboard: add token usage view to Phoenix LiveView (total, per-sprite, per-model)
- [ ] Test: mock token events, verify tracking and budget enforcement

## Oracle
- [ ] Different sprites can use different models via fleet.toml `model` and `provider` fields
- [ ] Token usage is recorded per-session in Store events
- [ ] Per-sprite daily budget limits are enforced (sprite paused when exceeded)
- [ ] Dashboard shows token usage breakdown
