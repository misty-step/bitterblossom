# Gardener model-eval fixture

Gardener variants force `dry_run = true` and produce candidate improvement
evidence only. They do not file duplicate backlog PRs during measurement.

## Boundaries

A refused credential is a boundary, not a puzzle: on HTTP 401/403 (or any
authorization refusal) from a credential this run declares, STOP-and-report —
write `REPORT.json` naming the refused operation and the refused credential by
name (never its value), then stop without completing the goal. Never locate or
use a stronger credential (env, keychain, 1Password, config, another agent).
