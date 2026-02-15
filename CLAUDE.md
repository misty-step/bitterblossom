# Bitterblossom CLAUDE.md

## Output Format Standardization

Issue #321: Unify output format flag to `--format=json|text` everywhere

### Current State

Commands with `--json` boolean flag (need migration):
- dispatch
- logs
- events
- compose
- watchdog
- agent

Commands with `--format` string flag (already correct):
- fleet
- status

Commands always emitting JSON (need `--format` with default json):
- provision
- teardown

Interactive commands (default text):
- add
- remove

### Migration Pattern

Reference implementations in fleet.go and status.go:
- Fleet uses `--format` with validation: format must be `json` or `text`
- Status uses same pattern
- Both default to `text`

For programmatic commands (provision, teardown), default to `json`.

### Hidden Alias Pattern

For backward compatibility, add deprecated `--json` as hidden alias:
```go
command.Flags().Bool("json", false, "Deprecated: use --format=json")
command.Flags().MarkHidden("json")
```

In the RunE function, check if `--json` was explicitly set and map it to `format="json"`.

### Default Format Rules

- Interactive commands (dispatch, logs, events, compose, watchdog, agent, add, remove): default to `text`
- Programmatic commands (provision, teardown): default to `json`

### Testing Requirements

Each command file has corresponding `_test.go` files:
- dispatch_test.go
- fleet_test.go
- status_test.go
- etc.

Tests should verify:
1. `--format=text` produces text output
2. `--format=json` produces valid JSON
3. `--json` (deprecated) still works and maps to json
4. Unknown format values return error
