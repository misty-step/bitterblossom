# check-env false-green on sprite CLI auth

Priority: critical
Status: ready
Estimate: S

## Goal
Make `check-env` fail when sprite operations will actually fail, instead of passing on `FLY_API_TOKEN` which the sprite CLI ignores.

## Problem
`Config.sprite_auth_available?/0` returns truthy when `FLY_API_TOKEN` is set. But all sprite operations (`exec`, `wake`, `probe`) use the `sprite` CLI, which requires org-level keyring auth via `sprite login`. `FLY_API_TOKEN` is never used by the sprite CLI.

Result: `check-env` passes → conductor starts → all wake attempts fail → all sprites degraded → HealthMonitor retries the same failing wake indefinitely.

## Evidence
Factory audit 2026-04-01: `FLY_API_TOKEN` set, `check-env` passed, all 3 sprites failed wake 3/3 attempts with "no token found for organization misty-step". Operator had to manually `sprite login` and restart.

## Sequence
- [ ] Add a live probe to `check-env`: attempt `sprite exec <any-sprite> printf ok` on one declared sprite
- [ ] If no sprites are declared yet, fall back to `sprite ls -o <org>` to verify CLI auth
- [ ] Remove `FLY_API_TOKEN` from `sprite_auth_available?/0` (it's not used by the CLI)
- [ ] If `FLY_API_TOKEN` should be supported, wire it through to the sprite CLI via `--token` flag or env

## Oracle
- [ ] `check-env` fails when sprite CLI keyring auth is expired, even if `FLY_API_TOKEN` is set
- [ ] `check-env` passes when sprite CLI keyring auth is valid
- [ ] `mix test` passes
- [ ] `mix compile --warnings-as-errors` passes
