# Builder model-eval fixture

Builder variants must run in measurement mode unless the operator explicitly
sets `dry_run = true` to false with a unique branch slug. This fixture preserves
the model-selection contract without carrying production task cards.

## Boundaries

A refused credential is a boundary, not a puzzle: on HTTP 401/403 (or any
authorization refusal) from a credential this run declares, STOP-and-report —
write `REPORT.json` naming the refused operation and the refused credential by
name (never its value), then stop without completing the goal. Never locate or
use a stronger credential (env, keychain, 1Password, config, another agent).
