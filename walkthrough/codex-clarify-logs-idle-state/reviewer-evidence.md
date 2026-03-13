# Reviewer Evidence: clarify `bb logs` idle state

## Merge Claim

`bb logs <sprite>` no longer strands operators on a vague `No active task` message. It now explains the observed idle state and points them to the next recovery command without polluting stdout.

## Why This Matters

The old message was technically correct but operationally weak. When an operator reaches for `bb logs`, they are usually debugging a sprite, not reading source. The command should say what it checked and what to do next.

## Before

Baseline from `HEAD:cmd/bb/logs.go`:

```go
func writeLogsNoTaskMsg(stderr io.Writer) error {
	_, err := fmt.Fprintf(stderr, "No active task\n")
	return err
}
```

## After

Current branch in `cmd/bb/logs.go`:

```go
func writeLogsNoTaskMsg(stderr io.Writer, spriteName string) error {
	_, err := fmt.Fprintf(stderr,
		"No active task on %q.\nThe sprite is reachable, but no agent is running and ralph.log is empty.\nTry: bb status %s\n",
		spriteName,
		spriteName,
	)
	return err
}
```

## Terminal Walkthrough

### 1. Full package check

```text
$ go test ./cmd/bb/...
ok  	github.com/misty-step/bitterblossom/cmd/bb	(cached)
```

### 2. Regression guard for the idle-state copy

```text
$ go test -run TestLogsNoActiveTaskGoesToStderr ./cmd/bb
ok  	github.com/misty-step/bitterblossom/cmd/bb	(cached)
```

### 3. Diff scope

```text
$ git diff --stat
 cmd/bb/logs.go      | 12 ++++++++----
 cmd/bb/logs_test.go | 13 ++++++++++---
 2 files changed, 18 insertions(+), 7 deletions(-)
```

## Persistent Verification

- `go test ./cmd/bb/...`
- `go test -run TestLogsNoActiveTaskGoesToStderr ./cmd/bb`

## Residual Gap

This branch improves the idle-state copy only. The `bb status` empty-workspace state still has room for a similar recovery hint, but that is intentionally out of scope here.
