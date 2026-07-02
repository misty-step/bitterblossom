# Model catalog watch commission fixture

## Goal

Compare runtime OpenRouter model references against the catalog fixture and
report whether the checked-in model evidence is stale.

## Oracle

The report names `fixture_drift`, `new_family_candidates`, and
`configured_successors`, and it distinguishes a no-drift result from a blocked
catalog read.

## Boundaries

Supported payload fields include `dry_run`: default `true` and
`file_backlog_pr`. Do not edit runtime agent configs, model-eval record files,
or catalog fixtures. A promotion requires a model-eval record and a reviewed PR.

## Output

Write `REPORT.json` with the model-catalog findings and residual risk.

## Receipt

The final answer repeats the `REPORT.json` summary and names any fixture or
runtime config path inspected.
