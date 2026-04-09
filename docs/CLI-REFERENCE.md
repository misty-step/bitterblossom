# CLI Reference

The supported operator surface is `mix conductor ...` from the `conductor/`
directory. Repo-level verification and landing stay local:

- `make ci-fast` for the fast Dagger lane
- `make ci` for the full Dagger lane
- `scripts/land.sh <branch>` for verdict validation and local squash landing

## Environment

Required:

- `dagger`
- Codex auth via `codex login` or `OPENAI_API_KEY`
- one of `SPRITE_TOKEN`, `FLY_API_TOKEN`, or a logged-in `sprite` CLI session

Optional, depending on the repo transport in use:

- remote git credentials for clone or publish operations

## Core Commands

### `mix conductor start --fleet ../fleet.toml`

Boot the full conductor pipeline against a fleet file.

### `mix conductor fleet [--fleet ../fleet.toml] [--reconcile]`

Show declared sprite health. With `--reconcile`, provision unhealthy sprites
before printing status.

Examples:

```bash
cd conductor
mix conductor fleet --fleet ../fleet.toml
mix conductor fleet --fleet ../fleet.toml --reconcile
```

### `mix conductor fleet audit [--fleet ../fleet.toml] [--json]`

Emit the fleet view as JSON, including summary counts for total sprites,
reachable sprites, healthy sprites, paused sprites, running sprites, and
available capacity. `running` is based on loop ownership, not on whether the
sprite happens to be inside a transient `codex` subprocess at that instant.

### `mix conductor status`

Alias for `mix conductor fleet status`. It does not require a previously
started local conductor process.

### `mix conductor sprite status <sprite> [--fleet ../fleet.toml] [--json]`

Inspect one declared sprite, including lifecycle status (`idle`, `running`,
`paused`, `draining`) and setup health. The JSON payload also carries `busy`
and `loop_alive` so operators can distinguish "owns a live loop" from
"actively executing an agent right now."

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

Read `${WORKSPACE}/ralph.log` for the active or most recent workspace on a
sprite.

Examples:

```bash
cd conductor
mix conductor logs bb-tansy
mix conductor logs bb-tansy --lines 50
mix conductor logs bb-tansy --follow
```

### `mix conductor sprite logs <sprite> [--follow] [--lines N]`

Alias for `mix conductor logs ...`.

### `mix conductor show-events [--limit N]`

Print recent events as JSON. The current CLI supports `--limit`; it does not
yet filter by `run_id`.

### `mix conductor check-env`

Validate local runtime prerequisites for the conductor surface.

### `mix conductor dashboard [--port 4000]`

Run the local LiveView dashboard.

## Local Verification And Landing

### `make ci-fast`

Run the fast Dagger verification lane from the repo root.

### `make ci`

Run the full Dagger verification lane from the repo root.

If you run this on a trusted CI runner, set `BB_ALLOW_PRIVILEGED_DAGGER_IN_CI=1`
so the wrapper can opt into Dagger's privileged engine path explicitly.

### `scripts/land.sh <branch> [--message "..."] [--push] [--delete-branch]`

Validate `refs/verdicts/<branch>`, run the full Dagger lane, and squash-land the
branch onto the default branch locally.

Landing requires a fresh `ship` verdict for the branch tip. Conditional or
stale verdicts must be refreshed first.

Verdict refs live under `refs/verdicts/<branch>`. Evidence bundles mirror them
under `.evidence/<branch>/<date>/`.

## Notes

- Sprite setup is no longer a separate Go CLI command.
- Stale agent recovery is handled by `Conductor.Sprite.kill/1` and by dispatch
  preflight.
- The historical `bb` transport no longer exists.
- Hosted remotes are transport only. Landing, review evidence, and verification
  are local-first concerns.
