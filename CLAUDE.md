# CLAUDE.md

Claude-family tools may read this file first. Keep it aligned w `AGENTS.md`.

Also read:
- `AGENTS.md` (canonical repo context)
- `docs/adr/001-claude-code-canonical-harness.md` (Claude Code decision)
- `docs/adr/002-architecture-minimalism.md` (thin CLI decision)

## What This Is

Bitterblossom = Go CLI `bb` that dispatches coding tasks to persistent AI sprites. Four core commands, one small ralph loop. Thin deterministic transport in Go; intelligence in Claude Code skills.

## Architecture

```text
cmd/bb/
  main.go               Cobra root, token exchange, helpers
  dispatch.go           Probe -> sync -> upload prompt -> run ralph
  logs.go               Tail + render ralph.log (pretty or --json)
  setup.go              Configure sprite: configs, persona, ralph, git auth
  status.go             Fleet overview or single sprite detail
  stream_json.go        stream-json renderer (shared by dispatch/logs)
  sprite_workspace.go   Find workspace on-sprite

scripts/
  ralph.sh                  The ralph loop: invoke agent, check signals, enforce limits
  ralph-prompt-template.md  Prompt template with {{TASK_DESCRIPTION}}, {{REPO}}, {{SPRITE_NAME}}
```

No `internal/` directory. No `pkg/`. All Go logic lives in `cmd/bb/`.

## Canonical Harness

Claude Code is the only supported sprite harness (ADR-001). Runtime is pinned to Sonnet 4.6 with official `ralph-loop` plugin enabled in settings.

```bash
# Direct
claude -p --dangerously-skip-permissions --verbose < prompt.md

# Via OpenRouter proxy (default sprite runtime)
ANTHROPIC_BASE_URL=https://openrouter.ai/api \
ANTHROPIC_AUTH_TOKEN="$OPENROUTER_API_KEY" \
ANTHROPIC_MODEL=anthropic/claude-sonnet-4-6 \
claude -p --dangerously-skip-permissions --verbose < prompt.md
```

NEVER set `ANTHROPIC_API_KEY` on sprites (billing risk).

## Build & Test

```bash
go build -o bin/bb ./cmd/bb
```

## Coding Standards

- Go 1.23+, `gofmt` + `golangci-lint`
- Semantic commits: `feat:`, `fix:`, `test:`, `docs:`, `refactor:`
- Handle errors explicitly (except `fmt.Fprintf` to stderr)
- No new packages. All Go code in `cmd/bb/`.
