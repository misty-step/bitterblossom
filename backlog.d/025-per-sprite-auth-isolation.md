# Per-sprite auth isolation — stop token contention cascade

Priority: critical
Status: ready
Estimate: M

## Goal
Each sprite must have its own auth session so that one sprite's token refresh does not invalidate other sprites' tokens. The current shared-token model causes a cascade where auth re-sync on sprite A invalidates sprite B's token, triggering B's re-sync which invalidates A's token, ad infinitum.

## Problem
All sprites receive the same ChatGPT OAuth refresh token (copied from the operator's `~/.codex/auth.json`). OAuth refresh tokens are single-use: when sprite A refreshes, the old token is invalidated, and sprite B's copy becomes stale. The auth self-healing (020) correctly detects and re-syncs, but the re-sync itself causes the next failure — creating an infinite cascade where the factory spends 100% of its cycles on auth recovery.

## Evidence
Factory overnight run 2026-04-02: events show `sprite_auth_failure` → `sprite_recovered` → `sprite_auth_failure` cycling every 4-6 minutes across bb-fixer and bb-polisher. No productive work completed. bb-builder backed off via rapid-exit backoff (correct behavior).

## Options

### Option A: Per-sprite Codex login (recommended)
Each sprite runs `codex login` independently during provision, creating its own OAuth session. The conductor does NOT sync from operator's auth cache.

- Pro: Each sprite has independent token lifecycle
- Pro: No conductor changes needed beyond removing the auth sync
- Con: Requires each sprite to be able to complete OAuth flow (may need headless/device-code flow)

### Option B: API key per sprite
Use `OPENAI_API_KEY` instead of ChatGPT auth. Each sprite gets the same API key (keys don't rotate on use).

- Pro: Simple, no rotation issues
- Con: More expensive (no Pro Plan subsidy)
- Con: Requires cost tracking infrastructure (022)

### Option C: Token relay with mutex
Conductor acts as token broker: only one sprite refreshes at a time, and the fresh token is distributed to all sprites before any can use it.

- Pro: Keeps shared token model
- Con: Complex, adds a coordination bottleneck
- Con: Still single-use rotation — just serialized

## Sequence
- [ ] Investigate: can Codex do device-code OAuth flow for headless login on sprites?
- [ ] If yes: update provision to run `codex login` on each sprite independently, remove `force_sync_codex_auth`
- [ ] If no: fall back to Option B with API key, wire up basic token tracking
- [ ] Test: run 3 sprites simultaneously, verify no auth cascade
- [ ] Remove `force_sync_codex_auth` from Launcher preflight (or make it conditional on auth source)

## Oracle
- [ ] 3 sprites run simultaneously for >30 minutes without auth failures
- [ ] No `sprite_auth_failure` events in Store during a clean run
- [ ] `mix test` passes
