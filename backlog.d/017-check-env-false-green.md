# check-env false-green on sprite CLI auth

Priority: critical
Status: done
Estimate: S

## Goal
Make `check-env` fail when sprite operations will actually fail, instead of passing on `FLY_API_TOKEN` which the sprite CLI ignores.

## Problem
`check-env --fleet` validated sprite CLI auth at org scope (`sprite ls -o <org>`), but did not verify that real sprite operations were possible (`sprite exec` on declared sprites). That allowed false-greens where operator preflight passed while sprite operations still failed.

`FLY_API_TOKEN` is also not consumed by the sprite CLI and must not be treated as sufficient auth by itself.

## Evidence
Factory audit 2026-04-01: `FLY_API_TOKEN` set, `check-env` passed, all 3 sprites failed wake 3/3 attempts with "no token found for organization misty-step". Operator had to manually `sprite login` and restart.

## Sequence
- [x] Add a live probe to `check-env`: attempt `sprite exec <any-sprite> printf ok` on one declared sprite
- [x] If no sprites are declared yet, fall back to `sprite ls -o <org>` to verify CLI auth
- [x] Keep `FLY_API_TOKEN` excluded from `sprite_auth_available?/0` (it's not used by the CLI)
- [x] Ensure all preflight call sites (`start`, `fleet --reconcile`, `check-env --fleet`) pass declared sprites into auth checks

## Oracle
- [x] `check-env` fails when sprite CLI keyring auth is expired, even if `FLY_API_TOKEN` is set
- [x] `check-env` passes when sprite CLI keyring auth is valid
- [x] `mix test` passes
- [x] `mix compile --warnings-as-errors` passes

## What Was Built
- `Conductor.CLI` now forwards declared fleet sprites into environment preflight from all relevant paths: `start`, `fleet --reconcile`, and `check-env --fleet`.
- `Conductor.Config.check_env!/1` now performs sprite auth checks with context-aware behavior:
  - declared sprites present: probe one via `Conductor.Sprite.exec(..., "printf ok", ...)`
  - no declared sprites: fallback to `sprite ls -o <org>`
- Tests now cover:
  - declared-sprite exec probe success path
  - declared-sprite exec failure without silent fallback to org listing
  - no-sprite fallback to org listing
  - CLI forwarding of declared sprites for `check-env --fleet` and `fleet --reconcile`

## Verification
- [x] `cd conductor && mix test test/conductor/config_test.exs test/conductor/cli_fleet_test.exs`
- [x] `cd conductor && mix compile --warnings-as-errors`

## Workarounds
- None.
