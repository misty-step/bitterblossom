# Auth self-healing for ChatGPT OAuth token rotation

Priority: critical
Status: ready
Estimate: M

## Goal
Make the conductor detect and recover from Codex auth token failures without operator intervention. The factory must self-heal auth — a refresh_token_reused error should trigger re-provisioning, not a silent session death.

## Problem
Codex uses ChatGPT OAuth with single-use refresh tokens. When a token is refreshed by one process, all other processes sharing that token are invalidated. The conductor syncs auth from the operator's `~/.codex/auth.json` at provision time, but if the token rotates mid-session, the sprite's auth dies silently. The health monitor sees "loop exited" but doesn't classify it as an auth failure.

## Evidence
Factory audit 2026-04-02 run 4: bb-builder's Codex session died with `refresh_token_reused` error. No recovery attempted.

## Sequence
- [ ] Classify auth failures as a distinct exit type (not just "loop exited") — grep sprite logs for known auth error patterns
- [ ] On auth failure detection, re-provision auth (`force_sync_codex_auth`) before relaunch
- [ ] Ensure each sprite gets a fresh auth sync on every relaunch, not just initial provision
- [ ] Consider per-sprite auth sessions to avoid cross-sprite token invalidation
- [ ] Test: simulate auth failure, verify re-provision and relaunch

## Oracle
- [ ] A sprite whose Codex auth token is invalidated mid-session is detected and relaunched with fresh auth
- [ ] `mix test` passes
- [ ] `mix compile --warnings-as-errors` passes
