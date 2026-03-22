# Issue 747 Plan

## Problem

Fleet sprites can sit idle long enough to become unreachable. When the conductor tries to
probe or use them, `sprite exec` can fail on the WebSocket handshake and the fleet stays
degraded indefinitely.

## Acceptance Mapping

- Detect unreachable sprites and retry recovery with backoff.
- Log a clear operator-facing error and persist a fleet event after recovery is exhausted.
- Wake sprites from stopped or suspended state when the conductor probes or uses them.
- Run the wake path during fleet reconciliation before marking a sprite degraded.

## Slice

- Add a wake-safe `Conductor.Sprite` exec transport using `sprite exec --http-post`.
- Add a `Sprite.wake/2` helper and use HTTP POST probing for reachability checks.
- Extend `Conductor.Fleet.Reconciler` with bounded wake retries and operator-visible fleet events.
- Cover the recovery path with focused sprite and reconciler tests.

## Verification

- [x] Add failing tests for sprite wake/recovery during fleet reconciliation.
- [x] Add failing tests for operator-visible recovery exhaustion.
- [x] Implement sprite wake support and recovery-aware reconciliation.
- [x] Run targeted tests and format touched files.
