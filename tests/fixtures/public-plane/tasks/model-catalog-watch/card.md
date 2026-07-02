# Model catalog watch commission fixture

Supported payload fields include `dry_run`: default `true` and
`file_backlog_pr`. The agent writes `REPORT.json` summarizing OpenRouter
`fixture_drift`, `new_family_candidates`, and `configured_successors`.

Do not edit runtime agent configs, model-eval record files, or catalog
fixtures. A promotion requires a model-eval record and a reviewed PR.

