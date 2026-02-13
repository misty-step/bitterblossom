# Task: Fix bb logs to show actual agent output during dispatch

You are working on `/Users/yawgmoth/repos/bitterblossom` (Go project). Branch: `kaylee/dispatch-reliability-scaffolding`.

## Problem

`bb logs <sprite>` reads from `logs/agent.jsonl` (structured events only), but the actual agent output goes to `ralph.log`. During an active dispatch, `bb logs` returns empty because the JSONL events are sparse — only heartbeats and progress. Users need to see what the agent is actually doing.

## Changes needed

### 1. Add `--raw` flag to `cmd/bb/logs.go`

Add a new `--raw` boolean flag. When set, `bb logs --raw <sprite>` reads `ralph.log` instead of `agent.jsonl`. This shows the actual Claude output.

In `newLogsCmdWithDeps`, add:

```go
var rawMode bool
```

Add the flag:
```go
cmd.Flags().BoolVar(&rawMode, "raw", false, "show raw agent output (ralph.log) instead of structured events")
```

### 2. Add raw log fetch mode

When `rawMode` is true and we're in remote mode, change the behavior:

```go
if rawMode {
    return runRemoteRawLogs(ctx, stdout, stderr, cli, names, follow, pollInterval)
}
```

Add the implementation:

```go
const defaultRemoteRalphLog = "/home/sprite/workspace/ralph.log"

func runRemoteRawLogs(ctx context.Context, stdout, stderr io.Writer, cli sprite.SpriteCLI, names []string, follow bool, pollInterval time.Duration) error {
    for _, name := range names {
        if len(names) > 1 {
            fmt.Fprintf(stdout, "=== %s ===\n", name)
        }

        var cmd string
        if follow {
            cmd = fmt.Sprintf("tail -n 50 -f %s 2>/dev/null", defaultRemoteRalphLog)
        } else {
            cmd = fmt.Sprintf("tail -n 100 %s 2>/dev/null", defaultRemoteRalphLog)
        }

        out, err := cli.Exec(ctx, name, cmd, nil)
        if err != nil {
            fmt.Fprintf(stderr, "logs: fetch raw logs from %q: %v\n", name, err)
            continue
        }
        if strings.TrimSpace(out) != "" {
            fmt.Fprintln(stdout, out)
        } else {
            fmt.Fprintf(stdout, "(no ralph.log output yet for %s)\n", name)
        }
    }
    return nil
}
```

### 3. Make `--raw` the default behavior

Actually, since most users want to see what the agent is doing (not structured events), make raw the default and add `--events` for structured mode:

- Remove the `--raw` flag
- Add `--events` flag instead: `cmd.Flags().BoolVar(&eventsMode, "events", false, "show structured event log (agent.jsonl) instead of raw output")`
- Default behavior (no flag): show ralph.log
- `--events`: show agent.jsonl (current behavior)

This means the RunE function routes like:

```go
if isRemote && !eventsMode {
    return runRemoteRawLogs(ctx, stdout, stderr, cli, names, follow, pollInterval)
}
```

### 4. Add a test in `cmd/bb/logs_test.go` or `cmd/bb/logs_extra_test.go`

Add `TestLogsCmdRawDefault` that verifies the default remote mode reads ralph.log, not agent.jsonl.

Look at existing test patterns in `cmd/bb/logs_test.go` for the fake CLI setup.

## Instructions

1. Make changes to cmd/bb/logs.go
2. Run `go test ./cmd/bb/...` to see what breaks
3. Fix test failures
4. Add new test
5. Run full `go test ./...`
6. Commit: `feat(logs): default to raw agent output, add --events for structured logs`
7. Push to origin
