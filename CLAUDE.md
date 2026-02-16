# CLAUDE.md

Claude-family tools may read this file first. Keep it aligned w `AGENTS.md`.

Also read:
- `AGENTS.md` (canonical repo context)
- `docs/adr/001-claude-code-canonical-harness.md` (Claude Code decision)
- `docs/adr/002-architecture-minimalism.md` (thin CLI decision)

## What This Is

Bitterblossom = Go CLI `bb` that dispatches coding tasks to persistent AI sprites. Four commands, one 52-line ralph loop. Thin deterministic transport in Go; intelligence in Claude Code skills.

## Architecture

```text
cmd/bb/
  main.go          120 LOC  Cobra root, token exchange, helpers
  dispatch.go      195 LOC  Probe → sync → upload prompt → run ralph
  logs.go          120 LOC  Tail + render ralph.log (pretty or --json)
  setup.go         289 LOC  Configure sprite: configs, persona, ralph, git auth
  status.go        129 LOC  Fleet overview or single sprite detail
  stream_json.go   200 LOC  stream-json renderer (shared by dispatch/logs)
  sprite_workspace.go        Find workspace on-sprite

scripts/
  ralph.sh          52 LOC  The ralph loop: invoke agent, check signals, enforce limits
```

No `internal/` directory. No `pkg/`. All Go logic lives in `cmd/bb/`.

## Canonical Harness

Claude Code is canonical sprite harness (ADR-001). OpenCode available as alternative via `--harness opencode`.

```bash
# Direct
claude -p --dangerously-skip-permissions --verbose < prompt.md

# Via OpenRouter proxy
ANTHROPIC_BASE_URL=https://openrouter.ai/api \
ANTHROPIC_AUTH_TOKEN="$OPENROUTER_API_KEY" \
ANTHROPIC_MODEL=moonshotai/kimi-k2.5 \
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
