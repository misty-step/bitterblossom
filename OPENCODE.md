# OPENCODE.md — BB OpenCode Configuration

## Overview

Bitterblossom uses OpenCode as its sole agent harness on sprites. All coding agent tasks run through OpenCode + OpenRouter.

## Default Model

**Kimi K2.5 Thinking** (`moonshotai/kimi-k2.5-thinking`)

- 256K context window
- Multi-step tool use and reasoning
- Excellent coding capability
- Cost: ~$0.50/Mtok via OpenRouter

## Alternative Models

| Model | When to Use |
|-------|-------------|
| `moonshotai/kimi-k2.5` | Routine tasks, faster speed |
| `z-ai/glm-4.7` | Fast iteration, simple edits |
| `moonshotai/kimi-k2.5-thinking-turbo` | Complex tasks, faster output |

## Invocation Pattern

```bash
# Standard task dispatch
opencode run -m openrouter/moonshotai/kimi-k2.5-thinking \
  --agent coder \
  "Full task description with success criteria"

# Fast model for simple tasks
opencode run -m openrouter/z-ai/glm-4.7 \
  --agent coder \
  "Simple edit: add X to Y"
```

## Environment

Only one env var needed on sprites:

```bash
export OPENROUTER_API_KEY="sk-or-v1-..."
```

**Do NOT set:**
- `ANTHROPIC_API_KEY` — risk of accidental billing
- `ANTHROPIC_BASE_URL` — Claude Code only, not needed
- `ANTHROPIC_AUTH_TOKEN` — Claude Code only, not needed

## Agent Configuration

The coder agent is defined in `.opencode/agents/coder.md` with:
- Anti-analysis-paralysis rules (write code within 5 minutes)
- Git commit patterns (early, often, semantic)
- Test-first approach when applicable

## Why NOT Claude Code

Extensive testing (Feb 9, 2026) confirmed:
- Claude Code silently hangs with non-Anthropic models via OpenRouter
- Claude Code's Anthropic skin on OpenRouter only routes to Anthropic providers
- Claude Code's internal model validation blocks on non-standard model names
- Direct Moonshot endpoint also hangs in Claude Code's `-p` mode

OpenCode has none of these issues. It works with any OpenRouter model natively.

See `docs/SPRITE-ARCHITECTURE.md` for the full decision record.
