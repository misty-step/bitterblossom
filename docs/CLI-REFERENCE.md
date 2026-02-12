# CLI Reference

`bb` is the Bitterblossom fleet control plane. All commands emit JSON by default for agent consumption. Human-readable output is available via `--format text` or the absence of `--json`.

## Global Environment

| Variable | Purpose |
|----------|---------|
| `FLY_API_TOKEN` / `FLY_TOKEN` | API token (Sprites API uses Fly.io auth) |
| `FLY_ORG` | Organization slug (e.g. `misty-step`) |
| `SPRITE_CLI` | Path to `sprite` binary |
| `OPENROUTER_API_KEY` | Canonical OpenRouter key for settings rendering |
| `ANTHROPIC_AUTH_TOKEN` | Legacy auth fallback when `OPENROUTER_API_KEY` is unset |

> **Note:** Sprites are a standalone service at [sprites.dev](https://sprites.dev), not Fly.io Machines. The `FLY_*` env vars are used because Sprites API authenticates via Fly.io tokens.
>
> **Token creation:** `fly auth token` is deprecated. Use:
> `fly tokens create org -o misty-step -n bb-cli -x 720h`
>
> **Onboarding helper:** `./scripts/onboard.sh --app bitterblossom-dash --write .env.bb && source .env.bb`

---

## compose

Composition-driven fleet reconciliation.

```
bb compose <subcommand> [flags]
```

### Persistent Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--composition` | `compositions/v1.yaml` | Path to composition YAML |
| `--app` | `$FLY_APP` | Fly.io app name |
| `--token` | `$FLY_API_TOKEN` | Fly.io API token |
| `--api-url` | `https://api.sprites.dev` | Sprites API base URL |
| `--json` | `false` | Emit JSON output |

### compose diff

Show reconciliation actions without executing.

```bash
bb compose diff
bb compose diff --json
```

### compose apply

Apply reconciliation actions. Dry-run by default.

```bash
bb compose apply                # dry-run: preview actions
bb compose apply --execute      # apply actions
```

| Flag | Default | Description |
|------|---------|-------------|
| `--execute` | `false` | Execute reconciliation actions |

### compose status

Show current composition vs desired state.

```bash
bb compose status
bb compose status --json
```

Exit code `0` always. Output includes missing, extra, and drifted sprites.

---

## fleet

View registered sprites and reconcile fleet state with Fly.io.

```
bb fleet [flags]
```

### Examples

```bash
# List all registered sprites with status
bb fleet

# Machine-readable output
bb fleet --format json

# Create missing sprites from registry
bb fleet --sync

# Preview sync without making changes
bb fleet --sync --dry-run

# Full reconciliation: create missing, remove orphaned
bb fleet --sync --prune

# Preview destructive operations
bb fleet --sync --prune --dry-run
```

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--registry` | `~/.config/bb/registry.toml` | Path to registry TOML file |
| `--org` | `$FLY_ORG` | Fly.io organization |
| `--sprite-cli` | `$SPRITE_CLI` | Path to sprite CLI |
| `--composition` | `compositions/v1.yaml` | Path to composition YAML |
| `--sync` | `false` | Reconcile fleet state (create missing sprites) |
| `--prune` | `false` | Remove sprites not in registry (requires `--sync`) |
| `--dry-run` | `false` | Preview changes without applying them |
| `--format` | `text` | Output format: `json` or `text` |
| `--timeout` | `10m` | Command timeout |

### Status Values

| Status | Meaning |
|--------|---------|
| `running` | Sprite exists and is running |
| `not found` | Registered but doesn't exist in Fly.io |
| `orphaned` | Exists in Fly.io but not in registry |

### Sync Behavior

When `--sync` is enabled:
1. Compares registry entries against actual sprites in Fly.io
2. Creates missing sprites using standard provisioning
3. If `--prune` is set, prompts for confirmation before destroying orphaned sprites
4. Archives observations before destruction

### Exit Codes

Exit `0` on success. Exit `1` on any error or if sync operations fail.

---

## dispatch

Dispatch a task prompt to a sprite. **Dry-run by default.**

```
bb dispatch [sprite] [prompt] [flags]
```

### Examples

```bash
# Preview dispatch plan
bb dispatch bramble "Build the auth API"

# Execute dispatch
bb dispatch bramble "Build the auth API" --execute

# Issue-based prompt (no explicit prompt needed)
bb dispatch bramble --issue 186 --repo misty-step/bitterblossom --execute

# Auto-assign (pick first available sprite from registry)
bb dispatch --issue 186 --repo misty-step/bitterblossom --execute

# Ralph loop with file prompt
bb dispatch bramble --ralph --file prompts/refactor.md --execute

# With repo clone
bb dispatch bramble --repo misty-step/heartbeat "Write webhook tests" --execute

# Mount one or more skill directories into sprite workspace
bb dispatch bramble --issue 252 --repo misty-step/bitterblossom \
  --skill base/skills/bitterblossom-dispatch \
  --skill base/skills/bitterblossom-monitoring \
  --execute --wait

# JSON output for agent consumption
bb dispatch bramble "Fix the bug" --execute --json
```

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--repo` | | Repo to clone/pull (`org/repo` or URL) |
| `--file` | | Read prompt from file |
| `--skill` | | Path to skill directory or `SKILL.md` (repeatable). Mounted at `./skills/<name>/` on sprite. **Limits:** max 10 mounts, 100 files/skill, 10MB/skill, 1MB/file |
| `--ralph` | `false` | Start persistent Ralph loop |
| `--execute` | `false` | Execute dispatch (default is dry-run) |
| `--dry-run` | `true` | Preview dispatch plan |
| `--json` | `false` | Emit JSON output |
| `--app` | `$FLY_APP` | Fly app name |
| `--token` | `$FLY_API_TOKEN` | Fly API token |
| `--api-url` | `https://api.sprites.dev` | Sprites API URL |
| `--org` | `$FLY_ORG` | Sprite org |
| `--sprite-cli` | `$SPRITE_CLI` | Sprite CLI binary path |
| `--composition` | `compositions/v1.yaml` | Composition YAML |
| `--max-iterations` | `50` | Ralph loop iteration safety cap |
| `--max-tokens` | `200000` | Ralph stuck-loop token safety cap (only with `--ralph`) |
| `--max-time` | `30m` | Ralph stuck-loop runtime safety cap (only with `--ralph`) |
| `--webhook-url` | `$SPRITE_WEBHOOK_URL` | Optional webhook URL |
| `--issue` | | GitHub issue number (also enables IssuePrompt default when no prompt is provided) |
| `--skip-validation` | `false` | Skip pre-dispatch issue validation |
| `--strict` | `false` | Fail on any issue validation warning |
| `--registry` | `~/.config/bb/registry.toml` | Path to sprite registry file |
| `--registry-required` | `false` | Require sprite to exist in registry (fail if missing) |

### State Machine

```
pending → provisioning → ready → prompt_uploaded → running → completed
                                                          ↘ failed
```

Any state can transition to `failed` on error.

### Exit Codes

Standard (see [contracts](contracts.md)).

### Skill Mount Limits

The `--skill` flag enforces guardrails to prevent accidental performance/pathology cases:

| Limit | Default | Description |
|-------|---------|-------------|
| Max mounts | 10 | Maximum number of `--skill` flags per dispatch |
| Max files/skill | 100 | Maximum files per skill directory |
| Max bytes/skill | 10 MB | Total size limit per skill |
| Max file size | 1 MB | Individual file size limit |
| Skill name pattern | `^[a-z][a-z0-9-]*$` | Valid skill directory names (lowercase alphanumeric with hyphens) |

These limits are configurable programmatically via `resolveSkillLimits`. Violations produce deterministic errors with remediation hints.

---

## watchdog

Fleet health checks with optional auto-recovery. **Dry-run by default.**

```
bb watchdog [flags]
```

### Examples

```bash
# Check all sprites (dry-run)
bb watchdog

# Check specific sprites
bb watchdog --sprite bramble --sprite fern

# Execute redispatch on dead sprites
bb watchdog --execute

# JSON for monitoring integration
bb watchdog --json
```

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--sprite` | all | Specific sprite(s) to check |
| `--execute` | `false` | Execute redispatch actions |
| `--dry-run` | `true` | Preview watchdog actions |
| `--json` | `false` | Emit JSON output |
| `--stale-after` | `2h` | Duration before a sprite is classified stale |
| `--max-iterations` | `50` | MAX_ITERATIONS for redispatch recovery |
| `--org` | `$FLY_ORG` | Sprite org |
| `--sprite-cli` | `$SPRITE_CLI` | Sprite CLI binary path |

### Health States

| State | Meaning | Action |
|-------|---------|--------|
| `active` | Running, producing commits | None |
| `idle` | No agent, no task | None |
| `complete` | Task finished | Needs attention |
| `blocked` | BLOCKED.md present | Needs attention |
| `dead` | No agent but has task | Redispatch (if PROMPT.md exists) |
| `stale` | Running but no commits for >2h | Investigate |
| `error` | Probe failed | Needs attention |

### Exit Codes

Exit `1` if any sprite needs attention. Exit `0` if fleet is healthy.

---

## agent

On-sprite coding-agent supervisor. Runs on the sprite itself.

```
bb agent <subcommand> [flags]
```

### agent start

Start the supervisor daemon. Daemonizes by default; use `--foreground` for direct execution.

```bash
bb agent start --task-prompt "Build the API" --task-repo misty-step/heartbeat
bb agent start --task-prompt "Fix tests" --task-repo misty-step/api --foreground
bb agent start --agent codex --yolo --task-prompt "Refactor auth" --task-repo org/repo
```

| Flag | Env | Default | Description |
|------|-----|---------|-------------|
| `--sprite` | `BB_SPRITE` | hostname | Sprite name |
| `--repo-dir` | `BB_REPO_DIR` | `.` | Repository directory |
| `--agent` | `BB_AGENT` | `codex` | Agent kind: `codex`, `kimi-code`, `claude`, `opencode` |
| `--agent-command` | `BB_AGENT_COMMAND` | | Explicit agent executable |
| `--agent-flags` | `BB_AGENT_FLAGS` | | Comma-separated agent flags |
| `--model` | `BB_AGENT_MODEL` | | Model selection |
| `--yolo` | `BB_AGENT_YOLO` | `false` | Enable agent YOLO mode |
| `--full-auto` | `BB_AGENT_FULL_AUTO` | `false` | Enable full-auto mode |
| `--env` | `BB_AGENT_ENV` | | `KEY=VALUE` environment overrides |
| `--pass-env` | `BB_AGENT_PASS_ENV` | | Environment variables to pass through |
| `--issue-url` | `BB_TASK_ISSUE_URL` | | Task issue URL |
| `--task-prompt` | `BB_TASK_PROMPT` | | **Required.** Task prompt |
| `--task-repo` | `BB_TASK_REPO` | | **Required.** Task repository |
| `--task-branch` | `BB_TASK_BRANCH` | | Task branch |
| `--event-log` | `BB_EVENT_LOG` | `.bb/events.jsonl` | JSONL event log path |
| `--output-log` | `BB_OUTPUT_LOG` | `.bb/output.log` | Agent output log path |
| `--pid-file` | `BB_PID_FILE` | `.bb/supervisor.pid` | PID file path |
| `--state-file` | `BB_STATE_FILE` | `.bb/state.json` | State file path |
| `--heartbeat-interval` | `BB_HEARTBEAT_INTERVAL` | `30s` | Heartbeat interval |
| `--progress-interval` | `BB_PROGRESS_INTERVAL` | `60s` | Progress polling interval |
| `--stall-timeout` | `BB_STALL_TIMEOUT` | `30m` | Stall timeout |
| `--restart-delay` | `BB_RESTART_DELAY` | `5s` | Delay before restart |
| `--shutdown-grace` | `BB_SHUTDOWN_GRACE` | `10s` | Grace period before SIGKILL |
| `--foreground` | `BB_AGENT_FOREGROUND` | `false` | Run in foreground |

### agent stop

Gracefully stop the supervisor. Sends SIGTERM, waits, then SIGKILL.

```bash
bb agent stop
bb agent stop --timeout 30s
```

| Flag | Default | Description |
|------|---------|-------------|
| `--pid-file` | `.bb/supervisor.pid` | PID file path |
| `--timeout` | `15s` | Wait time before SIGKILL |

### agent status

Show supervisor state, task, and progress.

```bash
bb agent status
bb agent status --json
```

| Flag | Default | Description |
|------|---------|-------------|
| `--state-file` | `.bb/state.json` | State file path |
| `--pid-file` | `.bb/supervisor.pid` | PID file path |
| `--json` | `false` | JSON output |

### agent logs

Show agent output log.

```bash
bb agent logs
bb agent logs --lines 50
bb agent logs --follow
```

| Flag | Default | Description |
|------|---------|-------------|
| `--output-log` | `.bb/output.log` | Output log path |
| `--lines` | `100` | Number of tail lines |
| `--follow` | `false` | Follow appended output |

---

## provision

Provision sprites from a composition.

```
bb provision <sprite-name> [flags]
bb provision --all [flags]
```

### Examples

```bash
bb provision bramble
bb provision --all
bb provision --all --composition compositions/v2.yaml
```

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--composition` | `compositions/v1.yaml` | Path to composition YAML |
| `--all` | `false` | Provision all sprites from composition |
| `--org` | `$FLY_ORG` | Fly.io organization |
| `--sprite-cli` | `$SPRITE_CLI` | Path to sprite CLI |
| `--timeout` | `30m` | Command timeout |

---

## sync

Push base config and persona definitions to running sprites.

```
bb sync [sprite-name ...] [flags]
```

If no sprite names given, syncs all sprites from composition.

### Examples

```bash
bb sync                     # sync all
bb sync bramble             # sync one
bb sync bramble fern        # sync specific sprites
bb sync --base-only         # only shared config, skip personas
```

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--composition` | `compositions/v1.yaml` | Path to composition YAML |
| `--base-only` | `false` | Only sync shared base config |
| `--org` | `$FLY_ORG` | Fly.io organization |
| `--sprite-cli` | `$SPRITE_CLI` | Path to sprite CLI |
| `--timeout` | `30m` | Command timeout |

---

## status

Fleet overview or detailed sprite status.

```
bb status [sprite-name] [flags]
```

### Examples

```bash
bb status                           # fleet overview (JSON)
bb status --format text             # human-readable fleet overview
bb status --checkpoints             # include checkpoint listings (slower)
bb status bramble                   # detailed single sprite
bb status bramble --format text     # human-readable detail
```

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--composition` | `compositions/v1.yaml` | Path to composition YAML |
| `--org` | `$FLY_ORG` | Fly.io organization |
| `--sprite-cli` | `$SPRITE_CLI` | Path to sprite CLI |
| `--format` | `json` | Output format: `json` or `text` |
| `--checkpoints` | `false` | Fetch checkpoint listings (slower for large fleets) |
| `--timeout` | `2m` | Command timeout |

---

## teardown

Export sprite learnings and destroy the sprite. Prompts for confirmation unless `--force`.

```
bb teardown <sprite-name> [flags]
```

### Examples

```bash
bb teardown bramble
bb teardown bramble --force
bb teardown bramble --archive-dir /tmp/archives
```

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--archive-dir` | `observations/archives` | Archive directory for exported data |
| `--force` | `false` | Skip confirmation prompt |
| `--org` | `$FLY_ORG` | Fly.io organization |
| `--sprite-cli` | `$SPRITE_CLI` | Path to sprite CLI |
| `--timeout` | `5m` | Command timeout |

---

## watch

Real-time event stream dashboard.

```
bb watch --file <path> [flags]
```

### Examples

```bash
# Live dashboard
bb watch --file events.jsonl

# Filter by sprite
bb watch --file events.jsonl --sprite bramble --sprite fern

# JSON stream for piping
bb watch --file events.jsonl --json

# One-shot scan
bb watch --file events.jsonl --once

# Filter by severity
bb watch --file events.jsonl --severity critical,warning
```

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--file` | | **Required.** JSONL event file(s) |
| `--sprite` | all | Filter by sprite name |
| `--type` | all | Filter by event type |
| `--severity` | all | Filter signals: `info`, `warning`, `critical` |
| `--since` | | Events since duration or RFC3339 |
| `--until` | | Events until RFC3339 |
| `--json` | `false` | Emit JSONL output |
| `--poll-interval` | `250ms` | File tail polling interval |
| `--refresh` | `1s` | Dashboard refresh interval |
| `--start-at-end` | `true` | Ignore existing lines |
| `--once` | `false` | Scan once and exit |

---

## logs

Query historical JSONL event logs.

```
bb logs --file <path> [flags]
```

### Examples

```bash
# Read all events
bb logs --file events.jsonl

# Last hour, JSON format
bb logs --file events.jsonl --since 1h --json

# Follow new events
bb logs --file events.jsonl --follow

# Filter by sprite and type
bb logs --file events.jsonl --sprite bramble --type progress
```

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--file` | | **Required.** JSONL event file(s) |
| `--sprite` | all | Filter by sprite name |
| `--type` | all | Filter by event type |
| `--since` | | Events since duration or RFC3339 |
| `--until` | | Events until RFC3339 |
| `--follow` | `false` | Follow events as files grow |
| `--json` | `false` | Emit JSONL output |
| `--poll-interval` | `250ms` | File tail polling interval |

---

## events

Query structured event history from the local daily event store (`~/.config/bb/events/`).

```bash
bb events [flags]
```

### Examples

```bash
# All events from the local store
bb events

# Filter by sprite and event type
bb events --sprite bramble --type progress

# Filter by issue number
bb events --issue 13

# Time window
bb events --since 1h
bb events --since 2026-02-12T00:00:00Z --until 2026-02-12T01:00:00Z

# JSONL output
bb events --json
```

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--dir` | `~/.config/bb/events` | Event store directory |
| `--sprite` | all | Filter by sprite name |
| `--type` | all | Filter by event type |
| `--issue` | | Filter by issue number |
| `--since` | | Events since duration or RFC3339 |
| `--until` | | Events until RFC3339 |
| `--limit` | `1000` | Maximum events to return (0 = unlimited) |
| `--json` | `false` | Emit JSONL output |

---

## version

Print `bb` version.

```bash
bb version
```
