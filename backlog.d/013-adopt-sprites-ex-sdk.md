# Adopt sprites-ex Elixir SDK as transport layer

Priority: high
Status: ready
Estimate: M

## Goal
Replace `Shell.cmd("sprite", ...)` subprocess calls with the `superfly/sprites-ex` Elixir SDK. The conductor talks to the Sprites platform via native WebSocket instead of shelling out to the Go CLI. This eliminates stdin blocking, transport mode confusion, wake-retry fragility, and shell escaping bugs.

## Non-Goals
- Delete the `bb` Go CLI in this item (that's 005)
- Use every SDK feature (checkpoints, DNS policy, TCP proxy) — just the CRUD + exec surface
- Change the fleet.toml format

## Sequence
- [ ] Add `{:sprites, "~> x.x", github: "superfly/sprites-ex"}` to `mix.exs`
- [ ] Create `Conductor.Sprites` adapter module wrapping SDK calls: `create/2`, `destroy/1`, `exec/3`, `start/2`, `stop/1`, `status/1`, `logs/2`
- [ ] Replace `Sprite.exec/3` internals — swap `Shell.cmd("sprite", ["exec", ...])` for `Sprites.cmd(sprite, command)`
- [ ] Replace `Sprite.provision/2` — swap CLI `sprite create` for `Sprites.create(name, opts)`
- [ ] Replace `Sprite.probe/1` — swap CLI health probe for SDK status call
- [ ] Replace `Sprite.start_loop/3` and `stop_loop/1` — use SDK's async exec
- [ ] Replace `Sprite.logs/2` — use SDK's log streaming
- [ ] Remove `Shell.cmd` calls from `Sprite` module (Shell.ex itself stays for bootstrap scripts)
- [ ] Remove wake-retry logic from `Sprite.exec` — SDK handles cold-start natively
- [ ] Update `SpriteCLIAuth` if SDK uses different auth mechanism
- [ ] `mix test` green
- [ ] Test against live sprites: provision, exec, health probe, start/stop loop, logs

## Oracle
- [ ] `Sprite` module has zero `Shell.cmd("sprite", ...)` calls
- [ ] `mix deps.tree` shows `sprites` dependency
- [ ] `Sprite.exec/3` uses WebSocket transport (no subprocess)
- [ ] Provisioning a sprite from fleet.toml works end-to-end via SDK
- [ ] `sprite start` / `sprite stop` / `sprite logs` CLI commands work via SDK
- [ ] No `--http-post` transport mode anywhere in codebase
- [ ] `mix test` passes

## Notes
Scout found `superfly/sprites-ex` provides: `Sprites.cmd/2` (sync), `Sprites.spawn/2` (async streaming), `Sprites.create/2`, `Sprites.destroy/2`. This maps directly to our existing `Sprite` module surface.

Memory confirms the transport fragility: "never use `--http-post` for sprite exec; HTTP POST 502s on cold sprites." The SDK handles wake/retry natively over WebSocket.

Depends on 012 (kill orchestration layer) being done first — no point swapping transport under dead code.
