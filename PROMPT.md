Implement format flag standardization for issue #321:

Modify ALL bb commands to use --format=json|text with hidden --json alias.

FILES TO MODIFY:
1. cmd/bb/dispatch.go - Replace --json bool with --format string + hidden --json alias (default text)
2. cmd/bb/logs.go - Same pattern (default text)
3. cmd/bb/events.go - Same pattern (default text)
4. cmd/bb/compose.go - Same pattern (default text)
5. cmd/bb/watchdog.go - Same pattern (default text)
6. cmd/bb/agent.go - Same pattern (default text)
7. cmd/bb/provision.go - Add --format, default json
8. cmd/bb/teardown.go - Add --format, default json
9. cmd/bb/add.go - Add --format, default text
10. cmd/bb/remove.go - Add --format, default text

Use fleet.go as reference implementation for the pattern:
- Add opts.Format field with default value
- Validate format is 'json' or 'text' in RunE
- Add deprecated --json as hidden alias via MarkHidden
- Map deprecated --json usage to format='json' when Changed

TDD APPROACH:
1. First update tests for each command
2. Then implement the format handling
3. Run make test to verify all pass
4. Run make lint to verify no style issues

Specific implementation details:

For dispatch.go (currently has --json bool, default text):
```go
type dispatchOptions struct {
    // ... other fields
    Format string  // Add this
    JSON   bool    // Keep as hidden alias, mark deprecated
}
// In command setup:
command.Flags().StringVar(&opts.Format, "format", "text", "Output format: json|text")
command.Flags().BolVar(&opts.JSON, "json", false, "Deprecated: use --format=json")
command.Flags().MarkHidden("json")
// In RunE validation:
if cmd.Flags().Changed("json") {
    opts.Format = "json"
}
format := strings.ToLower(strings.TrimSpace(opts.Format))
if format != "json" && format != "text" {
    return errors.New("--format must be json or text")
}
```

For provision.go (currently always JSON, default json):
```go
// In opts:
Format string  // default "json"
// Validate same pattern
// Change output to respect format: contracts.WriteJSON for json, fmt.Fprint for text
```

Follow the exact pattern in fleet.go lines 50-60 and status.go lines 60-70.

ACCEPTANCE:
- All commands accept --format=json|text
- --json is deprecated hidden alias that still works
- Tests pass
- Lint passes
- Build passes
