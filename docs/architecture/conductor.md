# Conductor

The conductor is the workflow brain. It decides when work starts, when it is blocked, and when it is safe to merge. Built as an Elixir/OTP application.

Source: [`conductor/lib/conductor/`](../../conductor/lib/conductor/)

## Module Map

```mermaid
flowchart TD
    CLI["cli.ex\nCLI entrypoint (mix conductor ...)"]
    Orchestrator["orchestrator.ex\nPolling loop + run dispatch"]
    RunServer["run_server.ex\nPer-run GenServer state machine"]
    Store["store.ex\nSQLite persistence"]
    GitHub["github.ex\nGitHub operations via gh CLI"]
    Sprite["sprite.ex\nSprite exec + dispatch + artifact fetch"]
    Workspace["workspace.ex\nWorktree lifecycle on sprites"]
    Prompt["prompt.ex\nBuilder prompt construction"]
    Shell["shell.ex\nSubprocess execution with timeout"]
    Config["config.ex\nRuntime configuration"]
    Issue["issue.ex\nIssue struct + readiness checks"]

    CLI --> Orchestrator
    Orchestrator --> RunServer
    RunServer --> Store
    RunServer --> GitHub
    RunServer --> Sprite
    RunServer --> Workspace
    RunServer --> Prompt
    Sprite --> Shell
    Workspace --> Shell
    GitHub --> Shell
```

## Supervision Tree

```mermaid
flowchart TD
    App["Conductor.Application\nOTP root"]
    App --> Repo["Conductor.Repo\nEcto/SQLite"]
    App --> OrcSup["Conductor.Orchestrator\nGenServer"]
    App --> RunSup["Conductor.RunSupervisor\nDynamicSupervisor"]
    RunSup --> RS1["RunServer (issue A)"]
    RunSup --> RS2["RunServer (issue B)"]
```

Each issue gets its own `RunServer` under `RunSupervisor`. Restarts are `:temporary` (no automatic retry — failure is a terminal state).

## Run State Machine

```mermaid
stateDiagram-v2
    [*] --> pending
    pending --> building: lease acquired + workspace ready
    building --> governing: builder artifact ready (status=ready)
    building --> blocked: builder artifact blocked
    building --> failed: dispatch failed or artifact missing
    governing --> merged: CI green → squash merge
    governing --> blocked: CI timeout or merge failure
    merged --> [*]
    blocked --> [*]
    failed --> [*]
```

## Trace Bullet (RunServer lifecycle)

```mermaid
sequenceDiagram
    participant O as Orchestrator
    participant RS as RunServer
    participant S as Store (SQLite)
    participant SP as Sprite
    participant GH as GitHub

    O->>RS: start_link (via DynamicSupervisor)
    RS->>S: acquire_lease
    RS->>S: create_run (phase=building)
    RS->>SP: Workspace.prepare (git worktree)
    RS->>SP: Sprite.dispatch (Claude Code builder)
    SP-->>RS: {:ok, _} or {:error, ...}
    RS->>SP: Sprite.read_artifact
    RS->>S: update_run (pr_number, phase=governing)
    RS->>GH: checks_green? (poll loop)
    RS->>GH: merge_pr
    RS->>S: complete_run (merged)
    RS->>S: release_lease
    RS->>SP: Workspace.cleanup
```

## Key Interfaces

### Orchestrator

- `run_once(opts)` — run a single issue synchronously
- `start_loop(opts)` — start continuous polling loop
- `pick_worker/1` — round-robin worker selection

### Store

- `acquire_lease/3`, `release_lease/2` — exclusive run ownership
- `create_run/1`, `update_run/2`, `complete_run/3` — run lifecycle
- `record_event/3` — append-only event log
- `heartbeat_run/1` — keepalive for stale-run detection

### GitHub

- `get_issue/2`, `eligible_issues/2` — intake
- `checks_green?/2` — CI polling
- `merge_pr/2` — squash merge

### Sprite

- `dispatch/4` — run Claude Code on a sprite with prompt + repo
- `exec/3` — raw command execution with timeout
- `read_artifact/2` — fetch builder-result.json from sprite

## Persistence

Two durable truth surfaces:

| Surface | Location | Contents |
|---------|----------|----------|
| SQLite | `.bb/conductor.db` (on conductor host) | runs, leases, events |
| Event log | same DB events table | append-only audit trail |

GitHub remains the human conversation surface. SQLite is what the machine remembers.

## What This Module Should Not Become

- not a second transport CLI (that is `bb`'s job)
- not a generic fleet manager
- not a bag of shell heuristics with implied state
- not a peer-to-peer sprite chat layer

Stay deep: small operator surface, rich internal orchestration.
