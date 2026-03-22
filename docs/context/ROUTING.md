# Routing Context

Current routing and dispatch authority lives in the Elixir conductor.

## Start Here

- `conductor/lib/conductor/orchestrator.ex`
- `conductor/lib/conductor/run_server.ex`
- `conductor/lib/conductor/fleet/reconciler.ex`
- `conductor/lib/conductor/sprite.ex`

## Questions

| Question | Start Here |
|---|---|
| How are issues selected? | `conductor/lib/conductor/orchestrator.ex` |
| How is a run executed? | `conductor/lib/conductor/run_server.ex` |
| How are sprites provisioned? | `conductor/lib/conductor/fleet/reconciler.ex`, `conductor/lib/conductor/sprite.ex` |
| How are logs tailed? | `conductor/lib/conductor/sprite.ex`, `docs/CLI-REFERENCE.md` |
| How do completion signals work? | `docs/COMPLETION-PROTOCOL.md`, `conductor/lib/conductor/run_server.ex` |
