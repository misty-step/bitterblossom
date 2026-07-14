# CI diagnose commission fixture

## Goal

Diagnose one failed CI signal. Read `RUN.json` first and report `task` from `RUN.json`,
then read `EVENT.json`. This public fixture models
`check_suite.failed` handling without shipping a production allowlist.

## Oracle

Required output fields include `"event"`, `"task"`, `"repo"`, `"rev"`,
`"claim"`, `"evidence"`, `"suggested_next_run"`, `"cost_usd"`,
`"artifact_paths": ["REPORT.json"]`, and `"residual_risk"`.

## Boundaries

A refused credential is a boundary, not a puzzle: on HTTP 401/403 (or any
authorization refusal) from a credential this run declares, STOP-and-report —
write `REPORT.json` naming the refused operation and the refused credential by
name (never its value), then stop without completing the goal. Never locate or
use a stronger credential (env, keychain, 1Password, config, another agent).

The task writes `REPORT.json` only. It does not edit code, comment, merge,
deploy, park tasks, resolve runs, replay dead letters, or run a builder.

## Output

Write `REPORT.json` with the failed workflow claim, evidence, suggested next
run, and residual risk.

## Receipt

The final answer repeats the `REPORT.json` summary and names the run/check
evidence inspected.
