# Model catalog watch commission fixture

## Goal

Compare runtime OpenRouter model references against the catalog fixture and
report whether the checked-in model evidence is stale.

## Oracle

The report names `fixture_drift`, `new_family_candidates`, and
`configured_successors`, and it distinguishes a no-drift result from a blocked
catalog read.

## Boundaries

A refused credential is a boundary, not a puzzle: on HTTP 401/403 (or any
authorization refusal) from a credential this run declares, STOP-and-report —
write `REPORT.json` naming the refused operation and the refused credential by
name (never its value), then stop without completing the goal. Never locate or
use a stronger credential (env, keychain, 1Password, config, another agent).

Supported payload fields include `dry_run`: default `true` and
`file_backlog_pr`. Do not edit runtime agent configs, model-eval record files,
or catalog fixtures. A promotion requires a model-eval record and a reviewed PR.

## Output

Write `REPORT.json` with the model-catalog findings and residual risk.

## Receipt

The final answer repeats the `REPORT.json` summary and names any fixture or
runtime config path inspected.
