# Migration: Shell Scripts → Go CLI

Bitterblossom is migrating from shell scripts to a Go control plane (`bb` CLI). This document maps deprecated shell commands to their Go replacements and tracks deprecation status.

## Why

Shell scripts served well as v1 glue but hit limits:

- **Brittle process handling** — `nohup`, `pkill -f`, PID file races
- **No testability** — can't unit test `bash -c` pipelines
- **Injection risk** — crafted `--repo` URLs could break out of single-quoted `bash -c` strings (#26)
- **No machine contracts** — exit codes and output formats were ad hoc
- **N+1 remote calls** — `upload_dir` called `sprite exec` per file (#22)

The Go CLI provides: explicit state machines, dependency-injected tests, validated inputs, JSON/NDJSON contracts, and deterministic exit codes.

## Command Mapping

| Shell Script | Go Command | Status |
|-------------|-----------|--------|
| `./scripts/provision.sh <sprite>` | `bb provision <sprite>` | Ported |
| `./scripts/provision.sh --all` | `bb provision --all` | Ported |
| `./scripts/sync.sh` | `bb sync` | Ported |
| `./scripts/sync.sh <sprite>` | `bb sync <sprite>` | Ported |
| `./scripts/status.sh` | `bb status` | Ported |
| `./scripts/status.sh <sprite>` | `bb status <sprite>` | Ported |
| `./scripts/teardown.sh <sprite>` | `bb teardown <sprite>` | Ported |
| `./scripts/dispatch.sh <sprite> <prompt>` | `bb dispatch <sprite> <prompt> --execute` | Ported |
| `./scripts/dispatch.sh <sprite> --ralph <prompt>` | `bb dispatch <sprite> --ralph <prompt> --execute` | Ported |
| `./scripts/dispatch.sh <sprite> --file prompt.md` | `bb dispatch <sprite> --file prompt.md --execute` | Ported |
| `./scripts/dispatch.sh <sprite> --repo org/repo <prompt>` | `bb dispatch <sprite> --repo org/repo <prompt> --execute` | Ported |
| `./scripts/dispatch.sh <sprite> --status` | `bb watchdog --sprite <sprite>` | Ported (different model) |
| `./scripts/dispatch.sh <sprite> --stop` | `bb agent stop` (on-sprite) | Ported |
| `./scripts/tail-logs.sh <sprite> -n 120` | `bb agent logs --lines 120` (on-sprite) | Ported |
| `./scripts/tail-logs.sh <sprite> --follow` | `bb agent logs --follow` (on-sprite) | Ported |

### New Commands (no shell equivalent)

| Go Command | Purpose |
|-----------|---------|
| `bb compose diff` | Preview fleet reconciliation actions |
| `bb compose apply --execute` | Apply reconciliation (provision + teardown + update) |
| `bb compose status` | Composition vs actual state comparison |
| `bb watch --file events.jsonl` | Real-time event stream dashboard |
| `bb logs --file events.jsonl` | Historical event log queries |
| `bb agent start` | On-sprite supervisor daemon |
| `bb agent status` | Supervisor state and progress |

## Key Differences

### Dry-Run Default

All Go commands that modify state are dry-run by default. Pass `--execute` to apply changes.

```bash
# Shell: executes immediately
./scripts/dispatch.sh bramble "Build the API"

# Go: previews the plan
bb dispatch bramble "Build the API"

# Go: executes
bb dispatch bramble "Build the API" --execute
```

### JSON Output

Every Go command supports `--json` for machine consumption. Output follows the [contract envelope](contracts.md).

```bash
bb dispatch bramble "Build the API" --execute --json | jq '.data.state'
bb watchdog --json | jq '.data.summary.needs_attention'
bb status --format json | jq '.data.sprites'
```

### Composition-Driven Operations

Go commands use composition files as the source of truth for fleet topology. Shell scripts had implicit sprite knowledge.

```bash
# Shell: knew about sprites from lib.sh globals
./scripts/sync.sh

# Go: reads composition YAML
bb sync --composition compositions/v1.yaml
```

### Input Validation

Go commands validate all inputs before making remote calls. Shell scripts validated minimally.

- Sprite names: must match `^[a-z][a-z0-9-]*$`
- Repo format: `org/repo` validated per component; URLs parsed via `url.Parse`
- Prompt: rejects empty strings
- Flags: conflicting combinations produce clear errors

## Deprecation Timeline

| Phase | Shell Scripts | Go CLI | When |
|-------|-------------|--------|------|
| Current | Available, still work | Primary interface | Now |
| Next | Deprecated warnings in README | Sole documented interface | After #44 |
| Final | Removed from `scripts/` | Only implementation | After full parity confirmed |

Shell scripts remain in the repo for reference during the transition but are no longer the documented interface. New features are Go-only.

## Issues Resolved by Migration

| Issue | Problem | Resolution |
|-------|---------|------------|
| #20 | No provider abstraction in lib.sh | Go `sprite.SpriteCLI` interface |
| #22 | N+1 remote calls in upload_dir | Go batches operations |
| #26 | Command injection via crafted repo URL | Go validates via `url.Parse` + regex |
| #38 | Global COMPOSITION mutation in lib.sh | Go passes composition path as flag |
