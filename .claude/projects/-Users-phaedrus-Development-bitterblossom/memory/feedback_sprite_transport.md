---
name: sprite_transport_path
description: Sprites exec must use WebSocket transport, never HTTP POST — HTTP POST 502s on cold sprites
type: feedback
---

Never use `--http-post` transport for sprite exec in the conductor. The HTTP POST path goes through the sprites.app HTTP proxy which returns 502 on cold sprites. The WebSocket path (default) auto-wakes cold sprites transparently in <1s.

**Why:** The conductor's `Sprite.probe/2`, `Sprite.wake/2`, and the retry path in `Sprite.exec/3` all used `transport: :http_post`. This caused every cold sprite to appear unreachable, triggering cascading failures in the reconciler, HealthMonitor, and phase worker backoff. Removing `--http-post` and using WebSocket-only fixed all sprite connectivity issues.

**How to apply:** When writing any code that calls `Sprite.exec/3`, never pass `transport: :http_post`. The default `:websocket` handles cold starts, warm sprites, and running sprites correctly. If you see `--http-post` in sprite exec paths, remove it.
