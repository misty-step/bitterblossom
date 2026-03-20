# CLI Reference

The supported operator surface is `mix conductor ...` from the `conductor/` directory.

## Environment

Required:

- `GITHUB_TOKEN`
- one of `SPRITE_TOKEN`, `FLY_API_TOKEN`, or a logged-in `sprite` CLI session

## Core Commands

### `mix conductor start --fleet ../fleet.toml`

Boot the full conductor pipeline against a fleet file.

### `mix conductor pause`

Pause new dispatches without stopping the application.

### `mix conductor resume`

Resume dispatch after a pause.

### `mix conductor fleet [--fleet ../fleet.toml] [--reconcile]`

Show declared sprite health. With `--reconcile`, create any missing declared sprites, then provision unhealthy ones before printing status.

Examples:

```bash
cd conductor
mix conductor fleet --fleet ../fleet.toml
mix conductor fleet --fleet ../fleet.toml --reconcile
```

### `mix conductor logs <sprite> [--follow] [--lines N]`

Read `${WORKSPACE}/ralph.log` for the active or most recent workspace on a sprite.

Examples:

```bash
cd conductor
mix conductor logs bb-weaver
mix conductor logs bb-weaver --lines 50
mix conductor logs bb-weaver --follow
```

### `mix conductor show-runs [--limit N]`

Print recent runs as JSON.

### `mix conductor show-events --run_id <run-id>`

Print event history for one run as JSON.

### `mix conductor show-incidents --run_id <run-id>`

Print recorded incidents for one run as JSON.

### `mix conductor show-waivers --run_id <run-id>`

Print recorded waivers for one run as JSON.

### `mix conductor check-env`

Validate local runtime prerequisites.

### `mix conductor dashboard [--port 4000]`

Run the local LiveView dashboard.

## Notes

- Sprite setup is no longer a separate Go CLI command.
- Stale agent recovery is handled by `Conductor.Sprite.kill/1` and by dispatch preflight.
- The historical `bb` transport no longer exists.
