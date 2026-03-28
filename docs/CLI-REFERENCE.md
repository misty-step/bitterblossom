# CLI Reference

The supported operator surface is `mix conductor ...` from the `conductor/` directory.

## Environment

Required:

- `GITHUB_TOKEN`
- one of `SPRITE_TOKEN`, `FLY_API_TOKEN`, or a logged-in `sprite` CLI session

## Core Commands

### `mix conductor start --fleet ../fleet.toml`

Boot the full conductor pipeline against a fleet file.

### `mix conductor fleet [--fleet ../fleet.toml] [--reconcile]`

Show declared sprite health. With `--reconcile`, provision unhealthy sprites before printing status.

Examples:

```bash
cd conductor
mix conductor fleet --fleet ../fleet.toml
mix conductor fleet --fleet ../fleet.toml --reconcile
```

### `mix conductor status`

Show health for the currently loaded fleet in the active conductor application.

### `mix conductor logs <sprite> [--follow] [--lines N]`

Read `${WORKSPACE}/ralph.log` for the active or most recent workspace on a sprite.

Examples:

```bash
cd conductor
mix conductor logs bb-weaver
mix conductor logs bb-weaver --lines 50
mix conductor logs bb-weaver --follow
```

### `mix conductor show-events [--limit N]`

Print recent events as JSON. The current CLI supports `--limit`; it does not yet
filter by `run_id`.

### `mix conductor check-env`

Validate local runtime prerequisites.

### `mix conductor dashboard [--port 4000]`

Run the local LiveView dashboard.

## Notes

- Sprite setup is no longer a separate Go CLI command.
- Stale agent recovery is handled by `Conductor.Sprite.kill/1` and by dispatch preflight.
- The historical `bb` transport no longer exists.
