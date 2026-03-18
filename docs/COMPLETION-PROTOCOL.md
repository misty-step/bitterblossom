# Completion Protocol

Signal files that agents write to communicate task status back to the dispatch transport and conductor.

## Signal Files

| File | Meaning | Written by |
|------|---------|-----------|
| `TASK_COMPLETE` | Task finished successfully | Agent (canonical) |
| `TASK_COMPLETE.md` | Task finished successfully | Agent (legacy fallback) |
| `BLOCKED.md` | Agent cannot proceed | Agent |

### TASK_COMPLETE

The canonical filename is `TASK_COMPLETE` with **no file extension**. Prompt templates should instruct agents to use this exact name.

Detection code accepts both `TASK_COMPLETE` and `TASK_COMPLETE.md` because some agents (particularly Claude Code) add `.md` extensions automatically. This dual-check is a compatibility measure, not an invitation to use either name interchangeably.

Contents: freeform summary of what was done.

### BLOCKED.md

Always uses the `.md` extension. Contents: explanation of what's blocking progress.

## Lifecycle

1. **Pre-dispatch cleanup** (`cmd/bb/dispatch.go`): All signal files are removed before the agent starts to prevent false-positive detection from previous runs.

2. **Agent execution**: The agent works on the task. When done, it writes `TASK_COMPLETE` (success) or `BLOCKED.md` (stuck).

3. **Detection** (`cmd/bb/dispatch.go`): Dispatch checks for signal files after the agent run and during recovery. `TASK_COMPLETE` or `TASK_COMPLETE.md` → success. `BLOCKED.md` → blocked exit.

4. **Status check** (`cmd/bb/status.go`): Single sprite status reports which signal files are present.

## Off-Rails Recovery

When the off-rails detector fires (silence abort), dispatch performs a two-step recovery check before reporting failure:

1. **Signal check**: Look for `TASK_COMPLETE` / `TASK_COMPLETE.md`. If found, treat as success.
2. **Commit check**: Compare current HEAD against the pre-dispatch HEAD SHA to detect commits produced during this dispatch. If new commits exist, treat as success with a warning (agent was mid-task but couldn't signal cleanly).

The commit check is scoped to the current dispatch by capturing HEAD SHA before the agent starts. This prevents stale commits from a prior failed dispatch from triggering false successes. When SHA capture fails, the check falls back to comparing HEAD against `origin/master`/`origin/main` with a warning.

Exit code 4 indicates an off-rails abort where neither signal files nor new commits were found.

## Where Signal Knowledge Lives

The Go transport now keeps the workspace/signal contract in one place and reuses it across commands:

| System | What it checks | File |
|--------|---------------|------|
| Workspace contract | Canonical sprite paths, signal filenames, prompt/log filenames, cleanup/check shell snippets | `cmd/bb/workspace_contract.go` |
| Dispatch transport | Completion, blocked, and off-rails recovery checks | `cmd/bb/dispatch.go` |
| Dispatch cleanup | Removes stale signals before the agent starts via the shared contract helpers | `cmd/bb/dispatch.go` |
| Status check | Reports signal file presence | `cmd/bb/status.go` |
