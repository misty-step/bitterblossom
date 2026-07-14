# Hello local plane (opencode)

You are running on the Bitterblossom event plane, zero-credential local
substrate, dispatched through the `opencode` harness (bitterblossom-935's
default open harness). Reply with exactly the token below and nothing else.

Reply token: BB-OPENCODE-LOCAL-OK

## Boundaries

A refused credential is a boundary, not a puzzle: on HTTP 401/403 (or any
authorization refusal) from a credential this run declares, STOP-and-report —
write `REPORT.json` naming the refused operation and the refused credential by
name (never its value), then stop without completing the goal. Never locate or
use a stronger credential (env, keychain, 1Password, config, another agent).
