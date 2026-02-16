# AGENTS.md

Universal project context for all coding agents working on Bitterblossom.

## What This Is

Bitterblossom is a Go CLI (`bb`) that dispatches coding tasks to AI sprites on [Sprites.dev](https://sprites.dev). Three commands, ~785 lines of Go, one 52-line ralph loop. Thin deterministic transport in Go; intelligence in Claude Code skills.

See [ADR-002](docs/adr/002-architecture-minimalism.md) for why.

## Architecture

```
cmd/bb/
  main.go          120 LOC  Cobra root, token exchange, helpers
  dispatch.go      195 LOC  Probe → sync → upload prompt → run ralph
  setup.go         289 LOC  Configure sprite: configs, persona, ralph, git auth
  status.go        129 LOC  Fleet overview or single sprite detail

scripts/
  ralph.sh          52 LOC  The ralph loop: invoke agent, check signals, enforce limits
  ralph-prompt-template.md   Prompt template with {{TASK_DESCRIPTION}}, {{REPO}}, {{SPRITE_NAME}}

base/               Shared config pushed to every sprite (CLAUDE.md, hooks, skills, settings.json)
sprites/            Persona files per sprite (e.g. bramble.md, fern.md)
docs/               Architecture docs, ADRs, completion protocol
```

No `internal/` directory. No `pkg/`. All Go logic lives in `cmd/bb/`.

## CLI

Build: `go build -o bin/bb ./cmd/bb`

### dispatch

Send a task to a sprite via the ralph loop. Foreground, streaming.

```bash
bb dispatch <sprite> "<prompt>" --repo owner/repo [flags]
```

| Flag | Default | Description |
|------|---------|-------------|
| `--repo` | (required) | GitHub repo (owner/repo) |
| `--timeout` | `30m` | Max wall-clock time for ralph loop |
| `--max-iterations` | `50` | Max ralph loop iterations |
| `--harness` | `claude` | Agent harness: `claude` or `opencode` |
| `--model` | | Model for opencode harness (e.g. `moonshotai/kimi-k2.5`) |

Pipeline: probe (15s) → verify setup → kill stale processes → repo sync → clean signals → upload prompt → run ralph → verify work → exit code.

Exit codes: 0 = success, 1 = failure, 2 = blocked.

### setup

Configure a sprite with base configs, persona, and ralph loop.

```bash
bb setup <sprite> [--repo owner/repo] [--force]
```

| Flag | Default | Description |
|------|---------|-------------|
| `--repo` | | GitHub repo to clone |
| `--force` | `false` | Re-clone repo, overwrite configs |

Uploads: CLAUDE.md, settings.json (with OpenRouter key patched in), hooks, skills, commands, persona, ralph.sh, prompt template. Configures git auth. Installs OpenCode (non-fatal if fails).

### status

Fleet overview or detailed sprite status.

```bash
bb status [sprite]
```

No flags. Fleet mode probes all sprites concurrently (3s timeout each). Single sprite mode shows signals, git state, and recent PRs.

## Agent Configuration

### Canonical Harness: Claude Code (ADR-001)

```bash
claude -p --dangerously-skip-permissions --verbose < prompt.md
```

For non-Anthropic models via OpenRouter proxy (configured in `settings.json` during setup):
```bash
ANTHROPIC_BASE_URL=https://openrouter.ai/api \
ANTHROPIC_AUTH_TOKEN="$OPENROUTER_API_KEY" \
ANTHROPIC_MODEL=moonshotai/kimi-k2.5 \
claude -p --dangerously-skip-permissions --verbose < prompt.md
```

### Tested Models (via OpenRouter)

| Model | Harness | Result | Cost vs Sonnet |
|-------|---------|--------|----------------|
| claude-sonnet-4-5 | claude | SUCCESS | 1x (baseline) |
| z-ai/glm-5 | opencode | SUCCESS (PR #143) | ~10x cheaper |
| minimax/minimax-m2.5 | opencode | SUCCESS (PR #142) | ~30x cheaper |
| moonshotai/kimi-k2.5 | opencode | PARTIAL (hung before commit) | ~10x cheaper |

### Environment

```bash
# Required for all dispatch
export GITHUB_TOKEN="$(gh auth token)"
export FLY_API_TOKEN="$(fly tokens create org -o personal -n bb -x 720h)"

# Required for setup
export OPENROUTER_API_KEY="..."

# NEVER set ANTHROPIC_API_KEY on sprites (billing risk)
```

## Completion Protocol

Signal files that agents write to the workspace root:

| File | Meaning |
|------|---------|
| `TASK_COMPLETE` | Task finished successfully (canonical) |
| `TASK_COMPLETE.md` | Task finished successfully (legacy fallback) |
| `BLOCKED.md` | Agent cannot proceed |

Ralph loop checks for these between iterations. See [docs/COMPLETION-PROTOCOL.md](docs/COMPLETION-PROTOCOL.md).

## Sprite Lifecycle

1. **Setup** (`bb setup fern --repo misty-step/repo`) — one-time config push
2. **Dispatch** (`bb dispatch fern "task" --repo misty-step/repo`) — repeatable
3. **Status** (`bb status fern`) — check signals, git state, PRs
4. **Sleep** — automatic after idle, near-zero cost
5. **Wake** — instant on next dispatch

Sprites are persistent. Don't destroy them.

## Coding Standards

- Go 1.23+, `gofmt` + `golangci-lint`
- Semantic commits: `feat:`, `fix:`, `test:`, `docs:`, `refactor:`
- Handle errors explicitly — no `_` for error returns (except `fmt.Fprintf` to stderr)
- No new packages. All Go code in `cmd/bb/`.

## Important Rules

- **Sprites, not Machines.** Use sprites-go SDK, not Fly CLI.
- **Claude Code is canonical.** OpenCode is the alternative harness, not deprecated (see ADR-001).
- **Persistent, not ephemeral.** Setup once, dispatch forever.
- **Don't add Go commands.** If it needs judgment, write a skill (see ADR-002).
- **Ralph loop is sacred.** The 52-line script is core. Changes require careful review.
