# Routing Guide

Use this file to decide where to start reading or editing.

## Route By Task Type

### Intake, leases, run state, review waves, merge governance

Start with:

- [`conductor/lib/conductor/orchestrator.ex`](../../conductor/lib/conductor/orchestrator.ex)
- [`conductor/lib/conductor/run_server.ex`](../../conductor/lib/conductor/run_server.ex)
- [`docs/CONDUCTOR.md`](../CONDUCTOR.md)
- [`docs/architecture/conductor.md`](../architecture/conductor.md)
- [`ADR-003`](../adr/003-conductor-control-plane.md)
- [`ADR-004`](../adr/004-bounded-review-governance.md) *(proposed direction, not current behavior)*

Typical changes here:

- issue selection
- lease lifecycle / heartbeat logic
- builder or reviewer orchestration
- CI wait policy
- review-thread or trusted-external-review handling
- merge gates and reconcile behavior

### Sprite setup, auth, repo sync, prompt upload, logs, kill, status

Start with:

- [`cmd/bb/setup.go`](../../cmd/bb/setup.go)
- [`cmd/bb/dispatch.go`](../../cmd/bb/dispatch.go)
- [`cmd/bb/status.go`](../../cmd/bb/status.go)
- [`cmd/bb/logs.go`](../../cmd/bb/logs.go)
- [`cmd/bb/kill.go`](../../cmd/bb/kill.go)
- [`docs/CLI-REFERENCE.md`](../CLI-REFERENCE.md)
- [`docs/architecture/bb-cli.md`](../architecture/bb-cli.md)

Typical changes here:

- setup/bootstrap behavior
- auth/token resolution
- repo sync/default-branch handling
- PTY execution and log streaming
- operator recovery paths

### Dispatch execution, silence/off-rails detection, completion protocol

Start with:

- [`cmd/bb/dispatch.go`](../../cmd/bb/dispatch.go)
- [`cmd/bb/offrails.go`](../../cmd/bb/offrails.go)
- [`cmd/bb/stream_json.go`](../../cmd/bb/stream_json.go)
- [`docs/COMPLETION-PROTOCOL.md`](../COMPLETION-PROTOCOL.md)

Typical changes here:

- heartbeat cadence
- error-loop normalization
- signal-file semantics
- raw vs pretty stream-json parsing

### Builder/reviewer artifact shape and prompt contracts

Start with:

- [`scripts/builder-prompt-template.md`](../../scripts/builder-prompt-template.md)
- [`conductor/lib/conductor/prompt.ex`](../../conductor/lib/conductor/prompt.ex)
- [`docs/COMPLETION-PROTOCOL.md`](../COMPLETION-PROTOCOL.md)

Typical changes here:

- artifact schema
- required builder/reviewer output sections
- prompt wording that affects conductor parsing
- handoff boundaries between sprite and conductor

### Shared base config, hooks, and agent behavior

Start with:

- [`base/settings.json`](../../base/settings.json)
- [`base/hooks/*`](../../base/hooks/)
- [`base/CLAUDE.md`](../../base/CLAUDE.md)
- [`base/skills/*`](../../base/skills/) — guidance only; not authoritative for live CLI flags or command behavior

Typical changes here:

- safety guard behavior
- fast feedback hook behavior
- shared agent instructions
- runtime defaults that get pushed onto sprites

### Persona or worker-specialization changes

Start with:

- [`sprites/*.md`](../../sprites/)
- [`project.md`](../../project.md)
- [`AGENTS.md`](../../AGENTS.md)

## Misroutes To Avoid

Do **not** start from these when you need the current architecture contract:

- [`base/skills/*`](../../base/skills/) for exact live CLI flags
- [`glance.md`](../../glance.md) files for exact current behavior
- [`docs/archive/`](../archive/) for implementation truth
- legacy shell wrappers in [`scripts/`](../../scripts/) as if they define the live control plane
- [`compositions/`](../../compositions/) as if they are the current scheduler state model

## Verify Against Code

Before editing docs or behavior, verify assumptions against code when any of these are involved:

- `bb` flags or command names
- run phases / blocking reasons
- artifact paths under `.bb/conductor/`
- signal files (`TASK_COMPLETE`, `BLOCKED.md`, etc.)
- review-thread handling or trusted external review semantics
- worker isolation claims

## Handy Starting Points

| Goal | Best Starting Files |
|---|---|
| Add a new operator inspection surface | [`conductor/lib/conductor/cli.ex`](../../conductor/lib/conductor/cli.ex), [`docs/CONDUCTOR.md`](../CONDUCTOR.md) |
| Harden reviewer readiness or sprite repair | [`conductor/lib/conductor/orchestrator.ex`](../../conductor/lib/conductor/orchestrator.ex), [`cmd/bb/setup.go`](../../cmd/bb/setup.go), [`cmd/bb/dispatch.go`](../../cmd/bb/dispatch.go) |
| Change dispatch UX or output | [`cmd/bb/dispatch.go`](../../cmd/bb/dispatch.go), [`docs/CLI-REFERENCE.md`](../CLI-REFERENCE.md) |
| Change completion semantics | [`cmd/bb/dispatch.go`](../../cmd/bb/dispatch.go), [`docs/COMPLETION-PROTOCOL.md`](../COMPLETION-PROTOCOL.md), conductor prompt/state handling |
| Refresh stale architecture docs | [`project.md`](../../project.md), [`docs/architecture/*`](../architecture/), [`docs/CODEBASE_MAP.md`](../CODEBASE_MAP.md), [`docs/context/*`](./) |
