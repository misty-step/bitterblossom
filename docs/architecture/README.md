# Architecture

Bitterblossom has two core surfaces:

- `conductor/`: Elixir/OTP orchestrator — leases issues, dispatches builders, governs PRs, merges
- `cmd/bb/`: Go transport CLI (transitional — being absorbed into Elixir per #621)

This stack is intentionally small. The overview explains the full software-factory shape, then the drill-down docs explain each module.

## Map

- [System Overview](#system-overview)
- [Conductor (Elixir)](./conductor.md)
- [bb CLI Transport](./bb-cli.md)
- [Repo-local Skills](./skills.md)
- [Architecture Glance](./glance.md)
- [Codebase Map](../CODEBASE_MAP.md)
- [Context Index](../context/INDEX.md)

## System Overview

```mermaid
flowchart LR
    subgraph Inputs["Work Intake"]
        GH["GitHub Issues"]
        Ops["Human Operator"]
    end

    subgraph Control["Bitterblossom Control Plane"]
        Conductor["Conductor\nconductor/ (Elixir/OTP)"]
        DB["Run Store\nSQLite"]
        Events["Event Log\nappend-only events"]
    end

    subgraph Runtime["Transport + Execution"]
        BB["bb CLI\ncmd/bb/ (Go)"]
        Sprites["Persistent Sprites\n(worker VMs)"]
    end

    subgraph Review["Governance Surfaces"]
        PR["Pull Request"]
        CI["CI + Required Checks"]
    end

    GH --> Conductor
    Ops --> GH
    Ops --> Conductor
    Conductor --> DB
    Conductor --> Events
    Conductor --> BB
    BB --> Sprites
    Sprites --> PR
    PR --> CI
    CI --> Conductor
    Conductor --> GH
```

## Trace Bullet

```mermaid
sequenceDiagram
    participant GH as GitHub
    participant C as Conductor (Elixir)
    participant BB as bb CLI
    participant W as Builder Sprite

    GH->>C: eligible issue exists
    C->>C: acquire lease + create run
    C->>C: prepare git worktree on sprite
    C->>BB: dispatch builder (via Sprite.dispatch)
    BB->>W: sync repo + run Claude Code
    W-->>GH: push branch + open PR
    W-->>C: builder-result.json artifact
    C->>C: read artifact, enter governance
    C->>C: poll CI until green or timeout
    C->>GH: squash merge
    C->>C: release lease + finalize run
```

## Design Rules

- GitHub is the human-facing work ledger.
- SQLite + event log are the machine-facing truth.
- `bb` stays transport-sized; workflow judgment lives in the conductor.
- Sprites are persistent VMs; execution uses isolated per-run git worktrees.
- Merge is a governance decision, not a builder feeling.

## Drill Down

### Control Plane

[Conductor](./conductor.md) covers:

- OTP supervision tree and GenServer lifecycle
- run-state machine (pending → building → governing → terminal)
- governance: CI polling, merge, block/fail paths
- persistence: SQLite runs + leases + events

### Transport Edge

[bb CLI Transport](./bb-cli.md) covers:

- setup / dispatch / status / logs / kill responsibilities
- what dispatch actually does on-sprite
- how the conductor uses `bb` as a runtime adapter
- transitional status (CLI surface shrinking as Elixir takes over)

### Repo-local Skills

[Skills](./skills.md) covers:

- first-party autonomy skills shipped onto sprites
- Bitterblossom-specific dispatch and monitoring skills
- provisioning contract and skill update path
