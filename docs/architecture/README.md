# Architecture Overview

Bitterblossom is now a one-language control plane centered on the Elixir conductor.

## Primary Surfaces

- `conductor/`: orchestration, run state, fleet repair, merge governance
- `base/skills/`: skill library provisioned onto sprites
- `scripts/`: prompt templates and legacy helpers

## Key Flow

1. load `fleet.toml`
2. reconcile unhealthy sprites
3. lease an eligible GitHub issue
4. prepare a sprite worktree
5. dispatch the builder via `Conductor.Sprite.dispatch/4`
6. govern PR state, CI, and review evidence
7. merge or block truthfully

See:

- [docs/CONDUCTOR.md](../CONDUCTOR.md)
- [docs/CLI-REFERENCE.md](../CLI-REFERENCE.md)
- [docs/adr/004-elixir-conductor-architecture.md](../adr/004-elixir-conductor-architecture.md)
