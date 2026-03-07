# bb CLI Transport

`bb` is the deterministic edge. It knows how to talk to sprites, bootstrap them, stream output, and report state. It should not become the workflow brain.

Files:

- [`cmd/bb/setup.go`](../../cmd/bb/setup.go)
- [`cmd/bb/dispatch.go`](../../cmd/bb/dispatch.go)
- [`cmd/bb/status.go`](../../cmd/bb/status.go)
- [`cmd/bb/logs.go`](../../cmd/bb/logs.go)
- [`cmd/bb/kill.go`](../../cmd/bb/kill.go)

## Command Map

```mermaid
flowchart TD
    Root["bb"] --> Setup["setup\nbootstrap sprite"]
    Root --> Dispatch["dispatch\nrun Ralph loop"]
    Root --> Status["status\nshow fleet or sprite truth"]
    Root --> Logs["logs\nstream remote ralph.log"]
    Root --> Kill["kill\nterminate active ralph loop"]
```

## Dispatch Pipeline

```mermaid
flowchart LR
    Probe["probe sprite"] --> Config["verify setup"]
    Config --> Busy["check active ralph loop"]
    Busy --> Cleanup["kill stale agent processes"]
    Cleanup --> Sync["repo sync to default branch"]
    Sync --> Prompt["render + upload prompt"]
    Prompt --> Ralph["run ralph.sh"]
    Ralph --> Verify["verify work + PR state"]
    Verify --> Exit["exit code / optional CI wait"]
```

## Setup Pipeline

```mermaid
flowchart LR
    Probe["probe sprite"] --> Dirs["create remote dirs"]
    Dirs --> Base["upload CLAUDE/settings/hooks/skills"]
    Base --> Persona["upload persona"]
    Persona --> Ralph["upload ralph.sh + prompt template"]
    Ralph --> Git["configure git auth"]
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
    participant R as Ralph

    C->>BB: dispatch builder or reviewer task
    BB->>S: probe + sync + upload prompt
    BB->>R: exec ralph.sh
    R-->>BB: completion signal or artifact
    BB-->>C: process exit + artifact availability
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
