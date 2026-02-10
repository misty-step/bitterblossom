# CLAUDE.md

Claude-family tools may read this file first. Keep it aligned w `AGENTS.md`.

Also read:
- `AGENTS.md` (canonical repo context)
- `docs/adr/001-claude-code-canonical-harness.md` (decision record)

## What This Is
Bitterblossom = Go CLI `bb` for managing Sprites on Fly.io: fleet lifecycle, dispatch, monitoring, compositions.

## Canonical Harness (Feb 10 2026)
Claude Code is canonical sprite harness.
OpenCode deprecated for sprite dispatch.

Proxy provider lets Claude Code route thru OpenRouter to any model.

## Claude Code (direct)
```bash
claude --yolo "TASK"
```

## Claude Code via OpenRouter (Anthropic proxy)
```bash
ANTHROPIC_BASE_URL=https://openrouter.ai/api \
ANTHROPIC_AUTH_TOKEN="$OPENROUTER_API_KEY" \
ANTHROPIC_MODEL=moonshotai/kimi-k2.5 \
claude --yolo "TASK"
```

NEVER set `ANTHROPIC_API_KEY` on sprites (billing risk).
