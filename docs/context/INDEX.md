# Context Index

This folder exists so a fresh agent can get oriented without guessing from stale session memory.

## Read Order

Read in this order when you need the current architecture, not the historical one:

1. [`docs/CODEBASE_MAP.md`](../CODEBASE_MAP.md)
2. [`docs/architecture/README.md`](../architecture/README.md)
3. [`docs/architecture/conductor.md`](../architecture/conductor.md)
4. [`docs/architecture/bb-cli.md`](../architecture/bb-cli.md)
5. [`docs/CONDUCTOR.md`](../CONDUCTOR.md)
6. [`docs/CLI-REFERENCE.md`](../CLI-REFERENCE.md)
7. [`docs/COMPLETION-PROTOCOL.md`](../COMPLETION-PROTOCOL.md)
8. [`AGENTS.md`](../../AGENTS.md)
9. [`project.md`](../../project.md) *(vision, glossary, roadmap language)*
10. [`docs/adr/002-architecture-minimalism.md`](../adr/002-architecture-minimalism.md)
11. [`docs/adr/003-conductor-control-plane.md`](../adr/003-conductor-control-plane.md)
12. [`docs/adr/004-bounded-review-governance.md`](../adr/004-bounded-review-governance.md) *(proposed direction, not current behavior)*
13. Code entrypoints: [`scripts/conductor.py`](../../scripts/conductor.py), [`cmd/bb/*.go`](../../cmd/bb/), [`scripts/ralph.sh`](../../scripts/ralph.sh)

## Authority Ranking

When sources disagree, trust them in this order:

1. **Code**
2. **Focused current docs** ([`docs/CODEBASE_MAP.md`](../CODEBASE_MAP.md), [`docs/context/*`](./), [`docs/architecture/*`](../architecture/), [`docs/CONDUCTOR.md`](../CONDUCTOR.md), [`docs/CLI-REFERENCE.md`](../CLI-REFERENCE.md), [`docs/COMPLETION-PROTOCOL.md`](../COMPLETION-PROTOCOL.md))
3. **Repo framing docs** ([`AGENTS.md`](../../AGENTS.md), [`project.md`](../../project.md), [`README.md`](../../README.md))
4. **Accepted ADRs**
5. **Proposed ADRs / roadmap docs**
6. **Everything else**

## Fast Routing

- Control-plane logic, leases, runs, review waves, merge policy:
  - [`scripts/conductor.py`](../../scripts/conductor.py)
  - [`docs/CONDUCTOR.md`](../CONDUCTOR.md)
  - [`docs/architecture/conductor.md`](../architecture/conductor.md)
- Sprite setup, dispatch, logs, status, recovery:
  - [`cmd/bb/*.go`](../../cmd/bb/)
  - [`docs/CLI-REFERENCE.md`](../CLI-REFERENCE.md)
  - [`docs/architecture/bb-cli.md`](../architecture/bb-cli.md)
- Runtime loop / signal files / off-rails behavior:
  - [`scripts/ralph.sh`](../../scripts/ralph.sh)
  - [`cmd/bb/offrails.go`](../../cmd/bb/offrails.go)
  - [`cmd/bb/stream_json.go`](../../cmd/bb/stream_json.go)
  - [`docs/COMPLETION-PROTOCOL.md`](../COMPLETION-PROTOCOL.md)
- Shared runtime config / hooks / agent instructions:
  - [`base/settings.json`](../../base/settings.json)
  - [`base/hooks/*`](../../base/hooks/)
  - [`base/CLAUDE.md`](../../base/CLAUDE.md)
- Persona / worker specialization:
  - [`sprites/*.md`](../../sprites/)

## Use With Caution

These files and areas are still useful context, but they are not reliable truth for the live conductor-first architecture:

- [`docs/archive/`](../archive/)
- root [`glance.md`](../../glance.md)
- [`docs/glance.md`](../glance.md)
- [`scripts/glance.md`](../../scripts/glance.md)
- [`reports/glance.md`](../../reports/glance.md)
- [`cmd/bb/glance.md`](../../cmd/bb/glance.md)
- [`base/glance.md`](../../base/glance.md)
- [`QA.md`](../../QA.md)
- [`base/skills/*`](../../base/skills/) when you need exact live CLI flags or command behavior
- shell wrappers in [`scripts/`](../../scripts/) that preserve older workflows
- [`compositions/`](../../compositions/) — experimental hypotheses; not the authoritative scheduler input for the conductor loop

## Current Questions → Where To Look

| Question | Start Here |
|---|---|
| Why did a run block or fail? | [`scripts/conductor.py`](../../scripts/conductor.py), [`docs/CONDUCTOR.md`](../CONDUCTOR.md), `.bb/conductor.db`, `.bb/events.jsonl` |
| What does `bb dispatch` actually do? | [`cmd/bb/dispatch.go`](../../cmd/bb/dispatch.go), [`docs/architecture/bb-cli.md`](../architecture/bb-cli.md) |
| How does setup/bootstrap work? | [`cmd/bb/setup.go`](../../cmd/bb/setup.go), [`docs/CLI-REFERENCE.md`](../CLI-REFERENCE.md) |
| What files prove work completion? | [`docs/COMPLETION-PROTOCOL.md`](../COMPLETION-PROTOCOL.md), [`scripts/ralph.sh`](../../scripts/ralph.sh) |
| Where do review artifacts live? | [`scripts/conductor.py`](../../scripts/conductor.py), [`docs/CODEBASE_MAP.md`](../CODEBASE_MAP.md) |
| What is current vs planned? | [`project.md`](../../project.md), [`ADR-002`](../adr/002-architecture-minimalism.md), [`ADR-003`](../adr/003-conductor-control-plane.md), [`ADR-004`](../adr/004-bounded-review-governance.md) |

## Companion Files

- [`docs/CODEBASE_MAP.md`](../CODEBASE_MAP.md)
- [`docs/context/ROUTING.md`](./ROUTING.md)
- [`docs/context/DRIFT-WATCHLIST.md`](./DRIFT-WATCHLIST.md)
