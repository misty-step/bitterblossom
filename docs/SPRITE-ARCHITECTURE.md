# Bitterblossom Sprite Architecture

> **Decision Record — Updated February 10, 2026**  
> **Status:** APPROVED — Claude Code is the canonical sprite harness. OpenCode is deprecated.  
> **See:** `docs/adr/001-claude-code-canonical-harness.md` for the reversal decision.

---

## Core Principles

### 1. Sprites Are Persistent Agent Computers

Sprites are NOT ephemeral sandbox containers. They are persistent, stateful development environments that:

- Create in 1-2 seconds
- Have 100GB durable storage (object-storage backed)
- Auto-sleep after 30s of inactivity (near-zero cost when idle)
- Resume instantly when needed
- Support checkpoint/restore (~300ms) for safe experimentation

**Each BB agent personality gets its own sprite.** "Bramble" lives on a sprite named `bramble` with all of Bramble's repos, tools, configs, and env vars. When Bramble finishes a task, the sprite persists. When the next task comes in, Bramble's sprite wakes up and picks up where it left off.

### 2. Claude Code Is the Canonical Agent Harness

**Claude Code is the canonical harness for sprite dispatch.** OpenCode is deprecated.

The Feb 9 experimentation showed Claude Code hanging with non-Anthropic models when pointed directly at OpenRouter. The proxy provider (PR #136) solves this by properly translating requests:

| Config | Result |
|--------|--------|
| Claude Code + direct OpenRouter | ❌ Silent hang (Feb 9 finding) |
| Claude Code + Moonshot direct | ❌ Silent hang (Feb 9 finding) |
| **Claude Code + proxy provider + OpenRouter** | **✅ Works (PR #136)** |
| **Claude Code + Anthropic native** | **✅ Works** |
| OpenCode + OpenRouter | ✅ Works (but deprecated — stability issues) |

Claude Code has superior tool use, proven PTY dispatch (`--yolo`), and better ecosystem support. The proxy provider eliminates the model limitation that motivated the original OpenCode decision.

### 3. OpenRouter Is the Provider Layer

All model routing goes through OpenRouter:

```
BB → sprite exec → Claude Code → Proxy Provider → OpenRouter → Model Provider
                                                                  ├── Moonshot (Kimi K2.5, K2.5 Thinking)
                                                                  ├── z.ai (GLM 4.7)
                                                                  ├── Together/DeepInfra (open models)
                                                                  └── Anthropic (Claude, if needed)
```

Benefits:
- Single API key for all models
- Automatic provider failover
- Usage tracking in OpenRouter dashboard
- No direct provider API keys needed on sprites

---

## Sprite Lifecycle

### Phase 1: Spawn

```
BB receives task → Creates sprite (1-2s) → Names it after agent personality
```

```bash
sprite create bramble
```

### Phase 2: Bootstrap

BB injects environment and clones repos:

```bash
# Write proxy provider config for Claude Code
sprite exec -s bramble -- bash -c '
  cat >> ~/.bashrc << EOF
export ANTHROPIC_BASE_URL="https://openrouter.ai/api"
export ANTHROPIC_AUTH_TOKEN="$OPENROUTER_API_KEY"
export ANTHROPIC_MODEL="moonshotai/kimi-k2.5"
export ANTHROPIC_SMALL_FAST_MODEL="moonshotai/kimi-k2.5"
export CLAUDE_CODE_SUBAGENT_MODEL="moonshotai/kimi-k2.5"
EOF
  source ~/.bashrc
'

# Clone project repo
sprite exec -s bramble -- bash -c '
  git clone https://github.com/misty-step/PROJECT.git ~/work
  cd ~/work && go mod download  # or npm install, etc.
'
```

### Phase 3: Checkpoint

After bootstrap, checkpoint the clean state:

```bash
sprite exec -s bramble -- sprite-env checkpoints create
# → Checkpoint v1 (instant, ~300ms)
```

### Phase 4: Task Execution (Ralph Loop)

```bash
sprite exec -s bramble -- bash -c '
  source ~/.bashrc
  cd ~/work
  claude --yolo "Implement feature X: <full task description with success criteria>"
'
```

### Phase 5: Result Collection

```bash
# Check git status for changes
sprite exec -s bramble -- bash -c 'cd ~/work && git diff --stat'

# Push changes
sprite exec -s bramble -- bash -c 'cd ~/work && git push origin HEAD'

# Checkpoint successful state
sprite exec -s bramble -- sprite-env checkpoints create
```

### Phase 6: Sleep (Automatic)

Sprite auto-sleeps after 30s of inactivity. Near-zero cost. Wakes instantly for next task.

### Phase 7: Recovery (If Needed)

If an agent goes off the rails:

```bash
sprite exec -s bramble -- sprite-env checkpoints restore v1
# → Restores to clean state in ~1 second
```

---

## Model Configuration

### Primary Models

| Model | Use Case | Speed | Cost |
|-------|----------|-------|------|
| `moonshotai/kimi-k2.5-thinking` | Complex tasks, reasoning | Medium | ~$0.50/Mtok |
| `moonshotai/kimi-k2.5` | Routine coding | Fast | ~$0.45/Mtok |
| `z-ai/glm-4.7` | Fast tasks | Fast | ~$0.40/Mtok |

### Fallback Models (Free Tier)

| Model | Notes |
|-------|-------|
| `z-ai/glm-4.5-air:free` | Free, good for simple tasks |
| `qwen/qwen3-coder:free` | Free coding model |
| `deepseek/deepseek-r1-0528:free` | Free reasoning model |

### Configuration via Environment

Claude Code uses environment variables for proxy provider configuration:

```bash
ANTHROPIC_BASE_URL=https://openrouter.ai/api
ANTHROPIC_AUTH_TOKEN=$OPENROUTER_API_KEY
ANTHROPIC_MODEL=moonshotai/kimi-k2.5
```

---

## Environment Variables on Sprites

### Required (Proxy Provider Config)

```bash
export ANTHROPIC_BASE_URL="https://openrouter.ai/api"
export ANTHROPIC_AUTH_TOKEN="$OPENROUTER_API_KEY"
export ANTHROPIC_MODEL="moonshotai/kimi-k2.5"
export ANTHROPIC_SMALL_FAST_MODEL="moonshotai/kimi-k2.5"
export CLAUDE_CODE_SUBAGENT_MODEL="moonshotai/kimi-k2.5"
```

### DO NOT Set

```bash
# NEVER set this on sprites:
ANTHROPIC_API_KEY    # Risk of accidental Anthropic billing — use ANTHROPIC_AUTH_TOKEN with proxy instead
```

---

## What NOT To Do

### ❌ Never use OpenCode for sprite dispatch
OpenCode is deprecated. Use Claude Code with the proxy provider. See ADR-001.

### ❌ Never destroy sprites after tasks
Sprites are persistent. They auto-sleep for free. Destroying wastes the bootstrap work.

### ❌ Never pass API keys via sprite exec -env flags
The `-env` flag has unreliable propagation. Write to `.bashrc` or env files instead.

### ❌ Never set ANTHROPIC_API_KEY on sprites
Use `ANTHROPIC_AUTH_TOKEN` with the proxy provider. Direct API keys risk accidental Anthropic billing.

### ❌ Never treat sprites as stateless
They have 100GB persistent storage. Use it. Clone repos once, install deps once, checkpoint.

---

## Migration Checklist (Claude Code Restoration)

> The previous OpenCode migration checklist (Feb 9) is **cancelled**. The following replaces it.

- [x] Write ADR-001 reversing OpenCode-only decision
- [x] Update `AGENTS.md` to reflect Claude Code as canonical
- [x] Update `docs/SPRITE-ARCHITECTURE.md` to reflect Claude Code as canonical
- [ ] Ensure proxy provider (PR #136) is merged
- [ ] Remove OpenCode-specific config (`opencode.json`) from sprite bootstrap
- [ ] Update `internal/agent/config.go` to use Claude Code as default harness
- [ ] Update `internal/lifecycle/provision.go` to bootstrap Claude Code env vars
- [ ] Update `docs/CLI-REFERENCE.md` with Claude Code dispatch examples
- [ ] Update `docs/SECRETS.md` with proxy provider env vars

---

## References

- `research/sprites-deep-dive.md` — Sprite architecture analysis
- `research/ralph-loops-experiments.md` — Ralph loop experiment results
- `research/kimi-glm-claudecode-findings.md` — Why Claude Code can't use Kimi/GLM
- Fly.io Sprites blog: https://fly.io/blog/fly-sprites/
- OpenRouter docs: https://openrouter.ai/docs
