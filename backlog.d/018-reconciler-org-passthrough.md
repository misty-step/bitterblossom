# Reconciler must pass fleet.toml org to sprite operations

Priority: high
Status: ready
Estimate: S

## Goal
Ensure the reconciler uses `sprite.org` from the fleet declaration for all sprite operations instead of falling back to `Config.sprites_org!()`.

## Problem
The fleet loader parses `org` from fleet.toml into each sprite struct. But the reconciler's `wake_opts`, `check_health`, and `provision_and_verify` don't pass `sprite.org` to `Sprite.exec/3`. All operations fall through to `Config.sprites_org!()` → `sprite_cli_org!()`, which returns whatever the CLI's default org is — which may differ from fleet.toml.

## Evidence
Factory audit 2026-04-01 run 2: fleet.toml declares `org = "misty-step"` but CLI defaulted to `adminifi`. All wake attempts failed with wrong-org auth error despite fleet.toml being correct.

## Sequence
- [ ] Pass `org: sprite.org` in `wake_opts/2`
- [ ] Pass `org: sprite.org` in `check_health/2` → `status_fn` call
- [ ] Pass `org: sprite.org` in `provision_and_verify/2`
- [ ] Test: reconciler uses sprite.org, not Config fallback

## Oracle
- [ ] Reconciler wake/probe/provision uses the org from fleet.toml, not Config fallback
- [ ] `mix test` passes
- [ ] `mix compile --warnings-as-errors` passes
