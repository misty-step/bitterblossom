# Sprites in Bitterblossom

Bitterblossom uses [Sprites](https://sprites.dev) as the compute substrate for AI coding agents. Sprites are a standalone service — isolated Linux sandboxes with persistent filesystems, purpose-built for AI workloads. They are NOT Fly.io Machines. This document describes how BB provisions, manages, and monitors sprites.

## Overview

```
┌─────────────┐     ┌─────────────┐     ┌─────────────────┐
│   bb CLI    │────▶│   Dispatch  │────▶│     Sprite      │
│  (local)    │     │ (provisioning)│   │  (remote agent) │
└─────────────┘     └─────────────┘     └─────────────────┘
                           │                       │
                           ▼                       ▼
                    ┌─────────────┐        ┌─────────────┐
                    │ Sprites API │        │ bb agent    │
                    │api.sprites.dev│      │ supervisor  │
                    └─────────────┘        └─────────────┘
```

**Key Components:**
- **bb dispatch**: Provisions sprites and starts agents
- **bb agent**: On-sprite supervisor daemon
- **Sprite CLI**: Remote execution and file management

---

## How BB Uses Sprites

### The BB Agent Supervisor

Each sprite running a BB-managed agent has a **supervisor daemon** (`bb agent`) that:

1. **Manages agent lifecycle**: Starts, monitors, and restarts coding agents
2. **Tracks progress**: Polls for activity and detects stalls
3. **Maintains state**: Writes state files for external monitoring
4. **Logs events**: JSONL event stream for audit trails

The supervisor runs as a daemon on the sprite, separate from the agent process.

### Agent Kinds

BB supports three coding agents:

| Kind | CLI | Description |
|------|-----|-------------|
| `codex` | `codex` | OpenAI Codex CLI |
| `kimi-code` | `kimi-code` | Moonshot Kimi K2.5 |
| `claude` | `claude` | Anthropic Claude Code |

Configure via `--agent` flag or `BB_AGENT` environment variable.

---

## Provisioning Workflow

When you run `bb dispatch`, BB executes this workflow:

```
┌─────────────────┐
│  1. VALIDATE    │  Check sprite name, prompt, repo format
└────────┬────────┘
         ▼
┌─────────────────┐
│ 2. CHECK EXISTS │  Query api.sprites.dev for existing sprite
└────────┬────────┘
         │
    ┌────┴────┐
    │ Exists? │
    └────┬────┘
   Yes /    \ No
      ▼      ▼
┌────────┐  ┌─────────────────┐
│ SKIP   │  │ 3. PROVISION    │  Create via Sprites API
│        │  │                 │  Metadata: managed_by=bb.dispatch
└────────┘  └────────┬────────┘
                     ▼
┌─────────────────────────────────────────┐
│ 4. SETUP REPO                           │
│    - Clone/pull repo to /home/sprite/workspace
│    - Run: git fetch && git pull --ff-only
└─────────────────────────────────────────┘
                     ▼
┌─────────────────────────────────────────┐
│ 5. UPLOAD PROMPT                        │
│    - Write to /home/sprite/workspace/.dispatch-prompt.md
│    - (Ralph mode: /home/sprite/workspace/PROMPT.md)
└─────────────────────────────────────────┘
                     ▼
┌─────────────────────────────────────────┐
│ 6. WRITE STATUS                         │
│    - Create /home/sprite/workspace/STATUS.json
│    - Contains: repo, started, mode, task
└─────────────────────────────────────────┘
                     ▼
┌─────────────────────────────────────────┐
│ 7. START AGENT                          │
│    - One-shot: pipe prompt to claude -p
│    - Ralph: start sprite-agent daemon
└─────────────────────────────────────────┘
```

### Example: Provision and Dispatch

```bash
# Dry run (preview only)
bb dispatch my-sprite "Implement OAuth" --repo misty-step/project

# Execute the dispatch
bb dispatch my-sprite "Implement OAuth" --repo misty-step/project --execute

# With Ralph (persistent loop)
bb dispatch my-sprite "Fix all bugs" --repo misty-step/project --ralph --execute
```

---

## Agent Configuration

### Command-Line Flags

```bash
bb agent start \
  --sprite my-sprite \
  --repo-dir /home/sprite/workspace/project \
  --agent kimi-code \
  --model k2.5 \
  --yolo \
  --task-prompt "Implement feature X" \
  --task-repo misty-step/project \
  --task-branch feature-x
```

### Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `BB_SPRITE` | Sprite name | Hostname |
| `BB_REPO_DIR` | Repository directory | `.` |
| `BB_AGENT` | Agent kind | `codex` |
| `BB_AGENT_COMMAND` | Override agent executable | (kind default) |
| `BB_AGENT_FLAGS` | Comma-separated flags | - |
| `BB_AGENT_MODEL` | Model selection | - |
| `BB_AGENT_YOLO` | Enable --yolo mode | `false` |
| `BB_AGENT_FULL_AUTO` | Enable --full-auto mode | `false` |
| `BB_AGENT_ENV` | KEY=VALUE env overrides | - |
| `BB_AGENT_PASS_ENV` | Env vars to pass through | - |
| `BB_TASK_PROMPT` | Task prompt | (required) |
| `BB_TASK_REPO` | Task repository | (required) |
| `BB_TASK_BRANCH` | Git branch | - |
| `BB_TASK_ISSUE_URL` | Related issue URL | - |
| `BB_EVENT_LOG` | Event log path | `.bb-agent/events.jsonl` |
| `BB_OUTPUT_LOG` | Agent output path | `.bb-agent/agent.log` |
| `BB_PID_FILE` | Supervisor PID file | `.bb-agent/supervisor.pid` |
| `BB_STATE_FILE` | State file | `.bb-agent/state.json` |
| `BB_HEARTBEAT_INTERVAL` | Heartbeat interval | `30s` |
| `BB_PROGRESS_INTERVAL` | Progress poll interval | `10s` |
| `BB_STALL_TIMEOUT` | Stall detection timeout | `5m` |
| `BB_RESTART_DELAY` | Delay before restart | `5s` |
| `BB_SHUTDOWN_GRACE` | Grace period for shutdown | `10s` |
| `BB_AGENT_FOREGROUND` | Run in foreground | `false` |

---

## Kimi K2.5 Setup

Kimi K2.5 runs via Moonshot's OpenAI-compatible API. Set these environment variables:

```bash
# On the sprite, configure the environment
sprite exec -s my-sprite -- bash -c 'cat >> ~/.bashrc << EOF
export ANTHROPIC_BASE_URL=https://api.moonshot.cn/v1
export ANTHROPIC_API_KEY=your-moonshot-api-key
EOF'
```

### Dispatch with Kimi

```bash
# Pass environment through bb dispatch
bb dispatch my-sprite "Refactor authentication" \
  --repo misty-step/project \
  --execute \
  --env "ANTHROPIC_BASE_URL=https://api.moonshot.cn/v1,ANTHROPIC_API_KEY=$MOONSHOT_AI_API_KEY"
```

### Manual Agent Start with Kimi

```bash
# SSH to sprite and start agent manually
sprite console -s my-sprite

# Then on the sprite:
export ANTHROPIC_BASE_URL=https://api.moonshot.cn/v1
export ANTHROPIC_API_KEY=your-moonshot-api-key

bb agent start \
  --agent kimi-code \
  --model k2.5 \
  --task-prompt "Your task here" \
  --task-repo misty-step/project
```

**Note:** Kimi uses the `ANTHROPIC_*` variables because the API is OpenAI-compatible and BB's agent configuration uses this convention for all OpenAI-compatible endpoints.

---

## Monitoring

### Agent Status

```bash
# Check supervisor and agent state
bb agent status

# JSON output for programmatic use
bb agent status --json
```

Status output includes:
- `status`: running, stopped, stalled
- `sprite`: sprite name
- `task`: current task prompt
- `repo`: repository being worked on
- `supervisor_pid`: PID of supervisor process
- `agent_pid`: PID of agent process
- `restarts`: number of agent restarts
- `stalled`: whether stall was detected

### Agent Logs

```bash
# Show last 100 lines
bb agent logs

# Show last N lines
bb agent logs --lines 500

# Follow log output (tail -f)
bb agent logs --follow

# Combine with status
bb agent status && bb agent logs --follow
```

### Event Stream

Events are written to `BB_EVENT_LOG` (default: `.bb-agent/events.jsonl`):

```bash
# View recent events
tail -f .bb-agent/events.jsonl | jq .
```

Event types:
- `agent.started`: Agent process started
- `agent.progress`: Progress detected
- `agent.heartbeat`: Periodic heartbeat
- `agent.stall_detected`: No progress timeout
- `agent.restart`: Agent restarted after failure
- `agent.stopped`: Agent stopped

### Stop the Agent

```bash
# Graceful stop (SIGTERM, then SIGKILL after timeout)
bb agent stop

# Custom timeout
bb agent stop --timeout 30s
```

---

## Composition-Based Provisioning

Bitterblossom uses **compositions** to define sprite fleets:

```yaml
# compositions/v1.yaml
version: 1
name: "Fae Court v1"

sprites:
  bramble:
    definition: sprites/bramble.md
    preference: "Systems & Data"
    fallback: true

  willow:
    definition: sprites/willow.md
    preference: "Interface & Experience"
```

### Provisioning Metadata

When BB provisions from a composition, it adds metadata to the sprite:

```json
{
  "managed_by": "bb.dispatch",
  "persona": "bramble",
  "config_version": "1"
}
```

This metadata is used for routing decisions and tracking.

---

## Full End-to-End Example

### 1. Create a Sprite

```bash
sprite create bb-dev-01
```

### 2. Configure Environment

```bash
# Upload GitHub SSH key
sprite exec -s bb-dev-01 -- mkdir -p ~/.ssh
sprite exec -s bb-dev-01 -- bash -c 'cat > ~/.ssh/id_ed25519' <<< "$SSH_KEY"
sprite exec -s bb-dev-01 -- chmod 600 ~/.ssh/id_ed25519

# Configure Git
sprite exec -s bb-dev-01 -- git config --global user.email "bb@mistystep.io"
sprite exec -s bb-dev-01 -- git config --global user.name "Bitterblossom"

# Set up Kimi environment
sprite exec -s bb-dev-01 -- bash -c 'cat >> ~/.bashrc << EOF
export ANTHROPIC_BASE_URL=https://api.moonshot.cn/v1
export ANTHROPIC_API_KEY='"$MOONSHOT_AI_API_KEY"'
EOF'
```

### 3. Dispatch a Task

```bash
bb dispatch bb-dev-01 "Implement JWT authentication middleware" \
  --repo misty-step/bitterblossom \
  --execute
```

### 4. Monitor Progress

```bash
# Check status
bb agent status --sprite bb-dev-01

# Watch logs
bb agent logs --sprite bb-dev-01 --follow

# Check events
sprite exec -s bb-dev-01 -- tail -f /home/sprite/workspace/STATUS.json
```

### 5. Inspect Results

```bash
# Check what files changed
sprite exec -s bb-dev-01 -- bash -c 'cd /home/sprite/workspace/bitterblossom && git status'

# View specific changes
sprite exec -s bb-dev-01 -- bash -c 'cd /home/sprite/workspace/bitterblossom && git diff'
```

### 6. Clean Up

```bash
# Stop the agent
bb agent stop --sprite bb-dev-01

# Or destroy the sprite entirely
sprite destroy bb-dev-01
```

---

## Advanced: Direct Sprite Commands

### Execute Arbitrary Commands

```bash
# Run tests
sprite exec -s my-sprite -- bash -c 'cd /home/sprite/workspace/project && npm test'

# Check disk usage
sprite exec -s my-sprite -- df -h

# Interactive debugging
sprite console -s my-sprite
```

### File Operations

```bash
# Upload a file
sprite exec -s my-sprite -- bash -c 'cat > /tmp/patch.diff' < local-patch.diff

# Download a file (via exec + cat)
sprite exec -s my-sprite -- cat /home/sprite/workspace/project/output.log > local-output.log
```

### Checkpoints for Experimentation

```bash
# Save state before risky change
sprite checkpoint create -s my-sprite -comment "Before auth refactor"

# If it goes wrong, restore
sprite restore -s my-sprite v1
```

---

## Troubleshooting

### Agent Won't Start

```bash
# Check if supervisor already running
bb agent status

# Kill stale PID file if needed
rm .bb-agent/supervisor.pid

# Try foreground mode for debugging
bb agent start --foreground --task-prompt "test" --task-repo test/repo
```

### Sprite Not Responding

```bash
# Check sprite exists
sprite list

# Verify via API
sprite api -s my-sprite /status

# Try console access
sprite console -s my-sprite
```

### Git Authentication Fails

```bash
# Verify SSH key on sprite
sprite exec -s my-sprite -- cat ~/.ssh/id_ed25519.pub

# Test GitHub connection
sprite exec -s my-sprite -- ssh -T git@github.com

# Check Git config
sprite exec -s my-sprite -- git config --list
```

### Kimi/CodeX Returns API Errors

```bash
# Verify environment variables
sprite exec -s my-sprite -- env | grep -E 'ANTHROPIC|OPENAI'

# Test API directly
sprite exec -s my-sprite -- curl -H "Authorization: Bearer $ANTHROPIC_API_KEY" \
  https://api.moonshot.cn/v1/models
```

---

## See Also

- **Sprite CLI Reference**: Run `sprite --help`
- **API Documentation**: https://sprites.dev/api
- **Composition Examples**: `compositions/`
