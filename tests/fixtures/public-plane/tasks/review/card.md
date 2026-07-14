# Cerberus review commission fixture

## Goal

Run `cerberus review-pr` through the command harness. The wrapper reads
`RUN.json` and `EVENT.json`, derives the repo and PR, and writes `REPORT.json`.

## Oracle

The wrapper produces a Cerberus review report for the requested PR and records
whether a public GitHub comment was suppressed or posted by task policy.

## Boundaries

A refused credential is a boundary, not a puzzle: on HTTP 401/403 (or any
authorization refusal) from a credential this run declares, STOP-and-report —
write `REPORT.json` naming the refused operation and the refused credential by
name (never its value), then stop without completing the goal. Never locate or
use a stronger credential (env, keychain, 1Password, config, another agent).

Manual payloads may request measurement mode. Webhook payloads post only when
the task is exactly `review` and no dry-run override is present. Red lines: no
approvals, request-changes reviews, merges, pushes, or source edits from this
wrapper.

## Output

Write `REPORT.json` with the review verdict, findings, comment policy, and
residual risk.

## Receipt

The final answer repeats the `REPORT.json` summary and names the repo/PR read.
