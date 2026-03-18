# bb CLI Transport

`bb` is the deterministic edge. It knows how to talk to sprites, bootstrap them, stream output, and report state. It should not become the workflow brain.

Files:

- [`cmd/bb/setup.go`](../../cmd/bb/setup.go)
- [`cmd/bb/dispatch.go`](../../cmd/bb/dispatch.go)
- [`cmd/bb/dispatch_checks.go`](../../cmd/bb/dispatch_checks.go)
- [`cmd/bb/status.go`](../../cmd/bb/status.go)
- [`cmd/bb/logs.go`](../../cmd/bb/logs.go)
- [`cmd/bb/kill.go`](../../cmd/bb/kill.go)

## Command Map

```mermaid
flowchart TD
    Root["bb"] --> Setup["setup\nbootstrap sprite"]
    Root --> Dispatch["dispatch\nrun agent command"]
    Root --> Status["status\nshow fleet or sprite truth"]
    Root --> Logs["logs\nstream remote dispatch log"]
    Root --> Kill["kill\nterminate active agent"]
```

## Dispatch Pipeline

```mermaid
flowchart LR
    Probe["probe sprite"] --> Busy["check active agent"]
    Busy --> Cleanup["kill stale agent processes"]
    Cleanup --> Sync["repo sync to default branch"]
    Sync --> Prompt["render + upload prompt"]
    Prompt --> Agent["run agent command"]
    Agent --> Verify["verify work + PR state"]
    Verify --> Exit["exit code / optional CI wait"]
```

## Setup Pipeline

```mermaid
flowchart LR
    Probe["probe sprite"] --> Dirs["create remote dirs"]
    Dirs --> Base["upload CLAUDE/settings/hooks/skills"]
    Base --> Persona["upload persona"]
    Persona --> Git["configure git auth"]
    Git --> Repo["clone or repair repo checkout"]
    Repo --> Meta["write workspace metadata"]
```

## Why This Layer Exists

```mermaid
flowchart TD
    Need["Need long-running remote execution"] --> Timeout["local agent shell is not enough"]
    Need --> Stream["need PTY/log streaming"]
    Need --> FS["need remote file upload + artifact fetch"]
    Need --> Recovery["need operator recovery commands"]
    Timeout --> BB["bb transport"]
    Stream --> BB
    FS --> BB
    Recovery --> BB
```

## Relationship To The Conductor

The conductor calls `bb`; `bb` does not own the workflow.

```mermaid
sequenceDiagram
    participant C as Conductor
    participant BB as bb dispatch
    participant S as Sprite
    participant A as Agent

    C->>BB: dispatch builder or reviewer task
    BB->>S: probe + sync + upload prompt
    BB->>A: exec agent command in workspace
    A-->>BB: process exit + signal files
    BB-->>C: process exit + PR/signal availability
```

## Responsibility Boundary

`bb` should keep:

- auth and connectivity
- repo sync
- prompt upload
- PTY execution
- log/status/reporting
- workspace metadata contracts

`bb` should avoid:

- issue prioritization
- review policy
- CI/merge governance
- semantic routing logic

That split is the reason Bitterblossom is still understandable.
