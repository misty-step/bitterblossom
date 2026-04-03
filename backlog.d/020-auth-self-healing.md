# Auth self-healing for ChatGPT OAuth token rotation

Priority: critical
Status: done
Estimate: M

## Goal
Make the conductor detect and recover from Codex auth token failures without operator intervention. A `refresh_token_reused` error should trigger re-provisioning, not silent session death.

## Problem
Codex uses ChatGPT OAuth with single-use refresh tokens. When a token is refreshed by one process, all other processes sharing that token are invalidated. The conductor syncs auth from the operator's `~/.codex/auth.json` at provision time, but if the token rotates mid-session, the sprite's auth dies silently. The health monitor sees "loop exited" but doesn't classify it as an auth failure.

## Evidence
Factory audit 2026-04-02 run 4: bb-builder's Codex session died with `refresh_token_reused` error. No recovery attempted.

## Sequence
- [ ] Add auth failure detection in `Launcher.launch/3`: after `stop_loop` preflight, check sprite logs for known auth error patterns (`refresh_token_reused`, `Failed to refresh token`, `401`, `auth_error`)
- [ ] Create `Sprite.detect_auth_failure/2`: exec on sprite to grep the last N lines of `ralph.log` for auth error signatures, return `{:auth_failure, reason}` or `:ok`
- [ ] On auth failure detection: call `Sprite.force_sync_codex_auth/1` before relaunch (already exists but only runs on initial provision)
- [ ] Add auth failure as a distinct event type in Store: `sprite_auth_failure` with reason payload
- [ ] In `HealthMonitor` transition for `:healthy → :unhealthy`: if `detect_auth_failure` returns a match, emit `sprite_auth_failure` event and force auth re-sync before relaunch
- [ ] Test: mock a sprite whose log contains auth errors, verify re-sync is triggered

## Oracle
- [ ] A sprite whose Codex auth token is invalidated mid-session is detected within one health check cycle
- [ ] Auth re-sync (`force_sync_codex_auth`) runs before relaunch after auth failure
- [ ] `sprite_auth_failure` event is recorded in Store with the error reason
- [ ] `mix test` passes
- [ ] `mix compile --warnings-as-errors` passes
