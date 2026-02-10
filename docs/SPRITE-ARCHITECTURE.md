# Bitterblossom Sprite Architecture

> **Decision Record — February 9, 2026**  
> **Status:** APPROVED — OpenCode is the sole agent harness. Claude Code is deprecated.

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

### 2. OpenCode Is the Only Agent Harness

**Claude Code is permanently deprecated for sprite dispatch.**

After extensive experimentation (8 test configurations), Claude Code cannot reliably use non-Anthropic models:

| Config | Result |
|--------|--------|
| Claude Code + OpenRouter + Kimi K2.5 | ❌ Silent hang |
| Claude Code + Moonshot direct + Kimi | ❌ Silent hang |
| Claude Code + z.ai + GLM 4.7 | ⚠️ Requires z.ai key |
| **OpenCode + OpenRouter + Kimi K2.5** | **✅ Works** |
| **OpenCode + OpenRouter + GLM 4.7** | **✅ Works** |

OpenCode has native OpenRouter support with no model validation issues.

### 3. OpenRouter Is the Provider Layer

All model routing goes through OpenRouter:

```
BB → sprite exec → OpenCode → OpenRouter → Model Provider
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
# Write OpenRouter config
sprite exec -s bramble -- bash -c '
  echo "export OPENROUTER_API_KEY=sk-or-v1-..." >> ~/.bashrc
  source ~/.bashrc
'

# Clone project repo
sprite exec -s bramble -- bash -c '
  git clone https://github.com/misty-step/PROJECT.git ~/work
  cd ~/work && go mod download  # or npm install, etc.
'

# Write opencode.json
sprite exec -s bramble -- bash -c '
  cat > ~/work/opencode.json << EOF
  {
    "provider": "openrouter",
    "model": "moonshotai/kimi-k2.5-thinking",
    "agents": { "coder": ".opencode/agents/coder.md" }
  }
  EOF
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
  opencode run -m openrouter/moonshotai/kimi-k2.5-thinking \
    --agent coder \
    "Implement feature X: <full task description with success criteria>"
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

### Configuration in opencode.json

```json
{
  "provider": "openrouter",
  "model": "moonshotai/kimi-k2.5-thinking",
  "agents": {
    "coder": ".opencode/agents/coder.md"
  }
}
```

---

## Environment Variables on Sprites

### Required

```bash
export OPENROUTER_API_KEY="sk-or-v1-..."  # Single key for all models
```

### DO NOT Set

```bash
# NEVER set these on sprites:
ANTHROPIC_API_KEY    # Risk of accidental Anthropic billing
ANTHROPIC_BASE_URL   # Claude Code only, not needed for OpenCode
ANTHROPIC_AUTH_TOKEN  # Claude Code only
```

---

## What NOT To Do

### ❌ Never use Claude Code on sprites
Claude Code cannot reliably use non-Anthropic models. It silently hangs.

### ❌ Never destroy sprites after tasks
Sprites are persistent. They auto-sleep for free. Destroying wastes the bootstrap work.

### ❌ Never pass API keys via sprite exec -env flags
The `-env` flag has unreliable propagation. Write to `.bashrc` or env files instead.

### ❌ Never use Anthropic API keys on sprites
OpenRouter is the only provider layer. One key, all models, no accidental billing.

### ❌ Never treat sprites as stateless
They have 100GB persistent storage. Use it. Clone repos once, install deps once, checkpoint.

---

## Migration Checklist

- [ ] Remove all Claude Code dispatch paths from BB
- [ ] Remove `AgentClaudeCode` / `AgentKimi` agent kinds (replace with `AgentOpenCode`)
- [ ] Update `internal/agent/config.go` to only support OpenCode
- [ ] Update `internal/lifecycle/provision.go` for persistent sprite lifecycle
- [ ] Add checkpoint support to provision/dispatch flow
- [ ] Remove `CLAUDE.md` from BB repo
- [ ] Update `AGENTS.md` to reflect OpenCode-only architecture
- [ ] Update `OPENCODE.md` with model routing info
- [ ] Update `opencode.json` with Kimi K2.5 Thinking as default
- [ ] Add `.opencode/agents/coder.md` anti-analysis-paralysis rules
- [ ] Update `docs/CLI-REFERENCE.md`
- [ ] Update `docs/SECRETS.md` — only OPENROUTER_API_KEY needed
- [ ] File and track all migration issues on GitHub

---

## References

- `research/sprites-deep-dive.md` — Sprite architecture analysis
- `research/ralph-loops-experiments.md` — Ralph loop experiment results
- `research/kimi-glm-claudecode-findings.md` — Why Claude Code can't use Kimi/GLM
- Fly.io Sprites blog: https://fly.io/blog/fly-sprites/
- OpenRouter docs: https://openrouter.ai/docs
