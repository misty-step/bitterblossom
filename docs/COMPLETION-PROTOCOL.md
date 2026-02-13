# Completion Protocol

Signal files that agents write to communicate task status back to the dispatch and watchdog systems.

## Signal Files

| File | Meaning | Written by |
|------|---------|-----------|
| `TASK_COMPLETE` | Task finished successfully | Agent (canonical) |
| `TASK_COMPLETE.md` | Task finished successfully | Agent (legacy fallback) |
| `BLOCKED.md` | Agent cannot proceed | Agent |
| `PR_URL` | Dedicated PR URL file | Agent (optional) |
| `STATUS.json` | Dispatch metadata | Dispatcher |

### TASK_COMPLETE

The canonical filename is `TASK_COMPLETE` with **no file extension**. Prompt templates should instruct agents to use this exact name.

Detection code accepts both `TASK_COMPLETE` and `TASK_COMPLETE.md` because some agents (particularly Claude Code) add `.md` extensions automatically. This dual-check is a compatibility measure, not an invitation to use either name interchangeably.

Contents: freeform summary of what was done. May contain a GitHub PR URL, which the polling system extracts via regex.

### BLOCKED.md

Always uses the `.md` extension. Contents: explanation of what's blocking progress. The first 5 lines are extracted as a summary by the watchdog and status check systems.

### PR_URL

Optional. If present, takes priority over extracting a URL from TASK_COMPLETE. Contains a bare GitHub pull request URL.

### STATUS.json

Written by the dispatcher at dispatch time, not by the agent.

```json
{
  "repo": "owner/repo",
  "started": "2026-02-12T03:00:00Z",
  "mode": "oneshot",
  "task": "brief task description"
}
```

## Lifecycle

1. **Pre-dispatch cleanup**: All signal files are removed before agent start to prevent false-positive detection from previous runs. This is the fix for a P0 bug where stale `TASK_COMPLETE` markers caused `--wait` to report success immediately (PR #280).

2. **Agent execution**: The agent works on the task. When done, it writes `TASK_COMPLETE` (success) or `BLOCKED.md` (stuck).

3. **Detection**: The `--wait` polling loop and the fleet watchdog both check for signal files. Both accept either `TASK_COMPLETE` or `TASK_COMPLETE.md`.

4. **PR URL extraction**: First checks `PR_URL` file. If absent, greps for `https://github.com/.../pull/N` pattern in whichever TASK_COMPLETE variant exists.

## Where Signal Knowledge Lives

Signal file constants are defined in `internal/dispatch/dispatch.go`:

```go
const (
    SignalTaskComplete   = "TASK_COMPLETE"
    SignalTaskCompleteMD = "TASK_COMPLETE.md"
    SignalBlocked        = "BLOCKED.md"
)
```

All code that references signal filenames should use these constants rather than string literals.

## Consumers

| System | What it checks | File |
|--------|---------------|------|
| `--wait` polling | Completion + blocked + PR URL | `cmd/bb/dispatch.go` |
| Fleet watchdog | Completion + blocked (for state classification) | `internal/watchdog/watchdog.go` |
| Oneshot cleanup | Removes stale signals before dispatch | `internal/dispatch/dispatch.go` |
| Ralph cleanup | Removes stale signals before Ralph start | `internal/dispatch/dispatch.go` |
| sprite-agent.sh | Checks for completion between iterations | `scripts/sprite-agent.sh` |
| dispatch.sh | Status check and cleanup | `scripts/dispatch.sh` |
