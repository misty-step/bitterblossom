# CLI Reference

`bb` is the Bitterblossom sprite dispatch CLI. Four core commands (+ version).

## Environment

| Variable | Required | Purpose |
|----------|----------|---------|
| `FLY_API_TOKEN` | Always | Fly.io token for Sprites API auth |
| `SPRITE_TOKEN` | Alternative | Direct sprites token (skips FLY_API_TOKEN exchange) |
| `SPRITES_ORG` | If not `personal` | Sprites org slug for token exchange |
| `GITHUB_TOKEN` | For dispatch | Git operations on sprite |
| `OPENROUTER_API_KEY` | For setup | Baked into sprite settings.json |

Token creation:
```bash
fly tokens create org -o personal -n bb-cli -x 720h
```

---

## dispatch

Send a task to a sprite via the ralph loop. Runs foreground with streaming stdout/stderr.
If no remote output arrives for ~45s, dispatch emits a keepalive line (`[dispatch] no remote output...`) so operators can distinguish silence from a hung CLI.

```
bb dispatch <sprite> <prompt> --repo <owner/repo> [flags]
```

### Examples

```bash
# Basic dispatch
bb dispatch fern "Fix the login bug" --repo misty-step/webapp

# With timeout and iteration limit
bb dispatch bramble "Add user search" --repo misty-step/api --timeout 20m --max-iterations 30

# OpenCode harness with specific model
bb dispatch bramble "Write tests" --repo misty-step/api --harness opencode --model z-ai/glm-5
```

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--repo` | (required) | GitHub repo (`owner/repo`) |
| `--timeout` | `30m` | Max wall-clock time |
| `--max-iterations` | `50` | Max ralph loop iterations |
| `--harness` | `claude` | Agent harness: `claude` or `opencode` |
| `--model` | | Model for opencode harness |

### Pipeline

1. Probe connectivity (15s timeout)
2. Verify setup (`ralph.sh` exists)
3. Kill stale agent processes
4. Repo sync (pull latest on default branch)
5. Clean stale signal files
6. Render and upload prompt
7. Run ralph loop (foreground, streaming)
8. Verify work produced (commits, PRs)

### Exit Codes

| Code | Meaning |
|------|---------|
| 0 | Success |
| 1 | Failure (timeout, error, max iterations) |
| 2 | Blocked (BLOCKED.md written by agent) |

---

## setup

Configure a sprite with base configs, persona, and ralph loop. Run once per sprite per repo.

```
bb setup <sprite> [flags]
```

### Examples

```bash
# Setup with repo clone
bb setup fern --repo misty-step/webapp

# Re-setup (force overwrite)
bb setup fern --repo misty-step/webapp --force
```

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--repo` | | GitHub repo to clone |
| `--force` | `false` | Re-clone repo, overwrite configs |

### What It Does

1. Probe connectivity
2. Create directory structure (`~/.claude/`, workspace)
3. Upload base configs (CLAUDE.md, settings.json with OpenRouter key, hooks, skills, commands)
4. Upload persona file (`sprites/<name>.md` → `PERSONA.md`)
5. Upload ralph.sh and prompt template
6. Install OpenCode (non-fatal if fails)
7. Configure git auth (credential helper, user identity)
8. Clone repo (if `--repo` provided)

---

## status

Fleet overview or detailed single sprite status.

```
bb status [sprite]
```

### Fleet Mode

```bash
bb status
```

Lists all sprites with status and reachability (3s probe per sprite, concurrent).

```
SPRITE          STATUS     REACH    NOTE
------          ------     -----    ----
bramble         started    ok
fern            started    ok
thorn           started    no       unreachable
```

### Single Sprite Mode

```bash
bb status fern
```

Shows signal files, git branch, dirty files, recent commits, and open PRs.

---

## logs

Stream a sprite's agent output (reads `${WORKSPACE}/ralph.log` on-sprite).

```
bb logs <sprite> [--follow] [--lines N] [--json]
```

### Examples

```bash
# Dump all output
bb logs bramble

# Follow live output (Ctrl+C to stop)
bb logs bramble --follow

# Last 50 lines
bb logs bramble --lines 50

# Raw Claude Code stream-json events
bb logs bramble --follow --json
```

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--follow` | `false` | Tail live output |
| `--lines` | `0` | Last N lines (0 = all; follow defaults to 50) |
| `--json` | `false` | Raw Claude Code `stream-json` events |

If you upgraded `bb`, re-run `bb setup <sprite>` once to upload the updated `ralph.sh` (it creates/appends `ralph.log`).

---

## version

```bash
bb version
```

Prints `bb <version> (<commit>, <date>)`.
