# CLI Reference

The supported operator surface is `mix conductor ...` from the `conductor/`
directory. The current transition state is:

- `mix conductor start` still boots the legacy always-on conductor session
- `mix conductor fleet ...` and `mix conductor sprite ...` are the agent-first
  operator surface for truthful inspection and per-sprite lifecycle control

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

### `mix conductor fleet audit [--fleet ../fleet.toml] [--json]`

Emit the fleet view as JSON, including summary counts for total sprites,
reachable sprites, healthy sprites, paused sprites, running sprites, and
available capacity.

### `mix conductor status`

Alias for `mix conductor fleet status`. It does not require a previously
started local conductor process.

### `mix conductor sprite status <sprite> [--fleet ../fleet.toml] [--json]`

Inspect one declared sprite, including lifecycle status (`idle`, `running`,
`paused`, `draining`) and setup health.

### `mix conductor sprite start <sprite> [--fleet ../fleet.toml]`

Provision a declared sprite if needed, sync its persona, and launch its loop in
detached mode. The command returns after the remote loop has been started.

### `mix conductor sprite pause <sprite> [--fleet ../fleet.toml] [--wait]`

Mark a sprite paused so future loop starts are refused. With `--wait`, also
stop the current loop before returning.

### `mix conductor sprite resume <sprite> [--fleet ../fleet.toml]`

Remove the pause marker so the sprite can be started again.

### `mix conductor sprite stop <sprite> [--fleet ../fleet.toml]`

Stop the current loop without changing pause state.

### `mix conductor logs <sprite> [--follow] [--lines N]`

Read `${WORKSPACE}/ralph.log` for the active or most recent workspace on a sprite.

Examples:

```bash
cd conductor
mix conductor logs bb-weaver
mix conductor logs bb-weaver --lines 50
mix conductor logs bb-weaver --follow
```

### `mix conductor sprite logs <sprite> [--follow] [--lines N]`

Alias for `mix conductor logs ...`.

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
- `fleet.toml` still uses `[[sprite]]` entries in this sprint-1 slice. Template
  catalog, clone, create, destroy, and scale flows remain follow-up work.
