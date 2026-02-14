# Bitterblossom QA Runbook

Bitterblossom is a declarative sprite factory for provisioning and orchestrating Claude Code agent fleets on Sprites (sprites.dev). It's a Go CLI (`bb`) that manages sprite lifecycle, dispatch, and fleet reconciliation.

## Build & Test

### Build
```bash
make build
# Output: bin/bb

# Or directly:
go build -o bin/bb ./cmd/bb
```

### Test
```bash
make test
# Runs: go test ./... + scripts/test_legacy_wrappers.sh

# Unit tests only:
go test ./...
```

### Lint
```bash
make lint
# Runs: golangci-lint run
```

## Prerequisites

- Go 1.24+
- Sprites CLI (`sprite`) installed at `~/.local/bin/sprite`
- Fly.io API token (`FLY_API_TOKEN`)
- OpenRouter API key (`OPENROUTER_API_KEY`)
- GitHub CLI (`gh`) authenticated

### Environment Setup
```bash
# Onboard and get env exports
./scripts/onboard.sh --app <app-name> --write .env.bb
source .env.bb

# Create Fly.io token (fly auth token is deprecated)
fly tokens create org -o misty-step -n bb-cli -x 720h

# Set model key
export OPENROUTER_API_KEY="<openrouter-key>"

# Optional: set GitHub secret for cerberus
printf '%s' "$OPENROUTER_API_KEY" | gh secret set OPENROUTER_API_KEY --repo misty-step/<repo>
```

## Happy Path Testing

### 1. Provision a Sprite
Create a new sprite with base config and persona:
```bash
bb provision <sprite-name>
# Example:
bb provision bramble
```
Verify:
- Sprite created in Sprites (check `sprite list`)
- Base config (CLAUDE.md, settings.json) uploaded
- Persona (from `sprites/<name>/`) applied
- Claude Code initialized

### 2. Dispatch Work
Send a task to a sprite:
```bash
bb dispatch <sprite> "<prompt>" --execute
# Or from file:
bb dispatch bramble --file /tmp/task.md --execute

# With GitHub issue:
bb dispatch bramble --issue 123 --repo misty-step/bitterblossom --execute
```
Verify:
- Task sent to sprite
- Work completes successfully
- Results returned (or logged)

### 3. Monitor with Watchdog
Check fleet health and auto-recover:
```bash
bb watchdog
bb watchdog --sprite bramble
```
Verify:
- Running sprites detected
- Dead sprites identified
- Auto-recovery triggers for dead sprites

### 4. Fleet Management
View and reconcile registered sprites:
```bash
bb fleet                    # List all sprites with status
bb fleet --format json     # Machine-readable
bb fleet --sync             # Create missing sprites
bb fleet --sync --prune     # Remove orphaned sprites
```
Verify:
- Registry (~/.config/bb/registry.toml) matches actual sprites
- Missing sprites created
- Orphaned sprites identified (exist in Fly.io but not registry)

### 5. Composition Reconciliation
Apply composition changes:
```bash
bb compose diff             # Preview changes
bb compose apply --execute # Apply changes
bb compose status          # Current state
```
Verify:
- Desired composition matches actual fleet
- New sprites created for missing entries
- Extra sprites marked for removal

### 6. Logs & Status
```bash
bb logs <sprite>            # Stream logs
bb status <sprite>          # Sprite details
```

### 7. Deprovision
Destroy a sprite:
```bash
bb teardown <sprite>
# Or:
bb remove <sprite>
```
Verify:
- Sprite destroyed in Sprites
- Removed from registry
- Observations archived (if any)

## Edge Cases

### 1. Sprite Creation Failure
- Network timeout during provision
- Invalid base config
- Duplicate sprite name

### 2. Dispatch Failure
- Sprite not running (auto-sleep)
- Network timeout during dispatch
- Invalid task prompt

### 3. Concurrent Dispatches
- Multiple dispatches to same sprite
- Verify queueing or error handling

### 4. Network Timeout
- Long-running commands should have timeouts
- Verify `--timeout` flag works

### 5. Orphaned Sprites
- Sprite exists in Fly.io but not in registry
- `bb fleet --sync --prune` should handle this

### 6. Composition Drift
- Registry differs from composition
- Verify `bb compose apply` reconciles

### 7. Auto-Sleep
- Sprites sleep when idle (~$0 cost)
- Verify dispatch wakes sprite

## Regression Checks

### Dispatch with Ralph Loop
- Verify `--ralph` flag works for continuous operation
- Check `dispatch_test.go` for expected behavior

### Skill Mounting
- Skills should be mounted during dispatch
- Verify `base/skills/bitterblossom-dispatch` works

### Hook Execution
- Safety hooks in `base/hooks/` should run
- Run `python3 -m pytest -q` to test hooks

### Registry Management
- `bb add` / `bb remove` modify registry correctly
- Verify `registry.toml` stays consistent

### JSON Output
- All commands should emit valid JSON with `--json`
- Verify agent consumption works

### Environment Variables
- `FLY_API_TOKEN`, `FLY_ORG`, `OPENROUTER_API_KEY` honored
- Legacy `ANTHROPIC_AUTH_TOKEN` fallback works

## Common Issues

| Issue | Likely Cause | Fix |
|-------|--------------|-----|
| `sprite: command not found` | Sprite CLI not installed | Install from sprites.dev |
| `FLY_API_TOKEN: empty` | Token not set | Run onboard.sh or set env var |
| Sprite not found | Auto-sleep or deleted | Run `sprite start <name>` |
| Dispatch hangs | Ralph mode waiting | Use `--wait` or Ctrl+C |
| Fleet drift | Manual sprite creation | Run `bb fleet --sync` |

## Manual Testing Checklist

- [ ] `bb provision <name>` creates sprite with base config
- [ ] `bb dispatch` sends task and gets results
- [ ] `bb watchdog` detects health and recovers dead sprites
- [ ] `bb fleet` shows correct status (running/not found/orphaned)
- [ ] `bb compose diff/apply` reconciles composition
- [ ] `bb logs` streams output
- [ ] `bb teardown` destroys sprite and cleans registry
- [ ] `--json` flag produces valid JSON
- [ ] Forked/composed dispatches work correctly
- [ ] Hooks execute on relevant operations

## CLI Commands Reference

| Command | Purpose |
|---------|---------|
| `bb provision <sprite>` | Create and bootstrap a sprite |
| `bb dispatch <sprite> <prompt>` | Send work to a sprite |
| `bb fleet` | List registered sprites |
| `bb fleet --sync` | Reconcile fleet state |
| `bb compose diff` | Preview composition changes |
| `bb compose apply` | Apply composition changes |
| `bb watchdog` | Check fleet health |
| `bb logs <sprite>` | Stream sprite logs |
| `bb status <sprite>` | Show sprite details |
| `bb teardown <sprite>` | Destroy a sprite |
| `bb add <sprite>` | Add sprite to registry |
| `bb remove <sprite>` | Remove sprite from registry |
