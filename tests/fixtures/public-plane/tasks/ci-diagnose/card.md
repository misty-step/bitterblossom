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

The task writes `REPORT.json` only. It does not edit code, comment, merge,
deploy, park tasks, resolve runs, replay dead letters, or run a builder.

## Output

Write `REPORT.json` with the failed workflow claim, evidence, suggested next
run, and residual risk.

## Receipt

The final answer repeats the `REPORT.json` summary and names the run/check
evidence inspected.
