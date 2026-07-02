# ADR-006: Bitterblossom Skill Projection Has One Source

- **Status:** Accepted
- **Date:** 2026-07-02
- **Related:** backlog 053, backlog 076, `skills/bitterblossom/`

## Context

Agents consume Bitterblossom through the portable skill folder as much as
through the `bb` binary. If that skill is copied into Harness Kit or another
harness by hand, command recipes and JSON contracts drift from the runtime.
The failure mode is subtle: the repo gate can stay green while off-repo agents
learn stale commands.

## Decision

`skills/bitterblossom/` in this repository is the source of truth for the
portable Bitterblossom agent interface.

Consumers may project it by one of these no-drift mechanisms:

- a source entry in the consumer bootstrap that points at this folder;
- a symlink to this folder;
- an automated projection step that replaces the whole folder from this source.

Manual copied skill folders are not an accepted projection path. A consumer may
cache a projected copy for packaging, but the cache must be generated from this
folder and must not become the editing surface.

The repo-local dogfood skill is intentionally separate at
`.agents/skills/bb-dogfood/`. It may reference the portable skill, but it must
not create a second `bitterblossom` alias under `skills/`.

## Consequences

- `tests/skill_artifacts.rs` gates the source path and duplicate-alias
  invariant.
- Runtime command and schema changes must update `skills/bitterblossom/` in
  the same PR as the behavior change.
- Harness Kit or other consumers should treat their Bitterblossom skill copy
  as generated state unless they intentionally upstream the change here.
