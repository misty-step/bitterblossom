# Agent loop persistence across sessions

Priority: medium
Status: blocked
Estimate: L

## Goal
Agents currently run for session_timeout_minutes then stop. Need a mechanism to resume loops across sessions — either external cron re-dispatch, systemd service, or infrastructure-level restart-on-completion.

## Non-Goals
- Infinite sessions (timeout is a safety net)
- State persistence across loops (agents re-observe on each start)

## Oracle
- [ ] Bitterblossom can run continuously for 24h+ without manual intervention
- [ ] Session timeout triggers graceful shutdown + automatic re-launch
- [ ] Agent picks up where it left off (by observing repo state, not stored state)

## Notes
Blocked on deciding the mechanism: cron, systemd, or OTP restart strategy.
