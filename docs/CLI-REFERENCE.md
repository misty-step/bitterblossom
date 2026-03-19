# CLI Reference

`bb` is the Bitterblossom sprite transport CLI. Workflow orchestration lives in [docs/CONDUCTOR.md](./CONDUCTOR.md), not in `bb`.

## Authentication

`bb` and the conductor try three auth methods in order:

1. **`SPRITE_TOKEN`** (preferred) — direct sprites API token, no exchange needed
2. **`FLY_API_TOKEN`** — exchanged for a sprites token via the SDK
3. **Sprite CLI session** — reads `~/.sprites/sprites.json` and keychain (macOS)

The **preferred operator path** is sprite CLI login:

```bash
sprite auth login          # interactive login, stores token in keychain
sprite auth switch -o org  # select org (if not personal)
```

After login, both `bb` and `mix conductor check-env` accept the sprite CLI session
without any environment variables. This is the recommended path for local development.

For CI/automated environments, set `SPRITE_TOKEN` or `FLY_API_TOKEN` explicitly.

### Org resolution

The sprites org is resolved in order: `SPRITES_ORG` > `FLY_ORG` > sprite CLI config (`current_selection.org`).

### Environment variables

| Variable | Required | Purpose |
|----------|----------|---------|
| `SPRITE_TOKEN` | One of three | Direct sprites token (skips exchange) |
| `FLY_API_TOKEN` | One of three | Fly.io token, exchanged for sprites token |
| Sprite CLI login | One of three | `~/.sprites/sprites.json` + keychain |
| `SPRITES_ORG` | If not `personal` | Override sprites org slug |
| `GITHUB_TOKEN` | For dispatch | Git operations on sprite |
| `OPENROUTER_API_KEY` | For setup | Baked into sprite settings.json |

Fallback Fly token creation:
```bash
fly tokens create org -o personal -n bb-cli -x 720h
```

---

## kill

Terminate stale agent processes on a sprite so a blocked/aborted dispatch can be recovered.

```
bb kill <sprite>
```

### Examples

```bash
bb kill fern
```

### Behavior

1. Connects to the sprite.
2. Kills stale `claude`, `codex`, or `opencode` processes matching the dispatch runtime patterns.
3. Verifies those processes are gone before exiting.

### Exit

- `0` on success, including clean state.
- Non-zero if the sprite is unreachable or cleanup verification fails.

---

## dispatch

Send a task to a sprite via the remote agent runtime. Runs foreground with streaming stdout/stderr.
If no remote output arrives for ~45s, dispatch emits a keepalive line (`[dispatch] no remote output...`) so operators can distinguish silence from a hung CLI.

```
bb dispatch <sprite> <prompt> --repo <owner/repo> [flags]
```

### Examples

```bash
# Basic dispatch
bb dispatch fern "Fix the login bug" --repo misty-step/webapp

# With a custom timeout
bb dispatch bramble "Add user search" --repo misty-step/api --timeout 20m

# Claude Sonnet 4.6 runtime (default)
bb dispatch bramble "Write tests" --repo misty-step/api
```

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--repo` | (required) | GitHub repo (`owner/repo`) |
| `--timeout` | `30m` | Max wall-clock time |

### Pipeline

1. Probe connectivity (15s timeout)
2. Refuse overlapping dispatch if an agent process is already running
3. Kill stale agent processes
4. Repo sync (pull latest on default branch)
5. Clean stale signal files
6. Render and upload prompt
7. Run the agent (foreground, streaming)
8. Verify work produced (commits, PRs)

### Exit Codes

| Code | Meaning |
|------|---------|
| 0 | Success |
| 1 | Failure (timeout, error, max iterations) |
| 2 | Blocked (BLOCKED.md written by agent) |

---

## setup

Configure a sprite with base configs, persona, and the dispatch runtime. Run once per sprite per repo.

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
| `--persona` | first available in `sprites/` | Persona sprite name or file path |

### What It Does

1. Probe connectivity
2. Create directory structure (`~/.claude/`, workspace)
3. Upload base configs (CLAUDE.md, settings.json with OpenRouter key, hooks, skills, commands)
4. Upload persona file (`sprites/<name>.md` → `PERSONA.md`)
5. Upload the builder prompt template
6. Configure git auth (credential helper, user identity)
7. Clone repo (if `--repo` provided)
8. Write workspace metadata at `.bb/workspace.json`

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

Shows signal files, git branch, dirty files, recent commits, and PR visibility.

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

If you upgraded `bb`, re-run `bb setup <sprite>` once so the sprite gets the latest base configs, prompt template, and runtime metadata.

---

## version

```bash
bb version
```

Prints `bb <version> (<commit>, <date>)`.
