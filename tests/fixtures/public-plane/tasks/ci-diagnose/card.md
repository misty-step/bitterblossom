# CI diagnose commission fixture

Read `RUN.json` first and report `task` from `RUN.json`. Then read
`EVENT.json`. This public fixture models `check_suite.failed` handling without
shipping a production allowlist.

Required output fields include `"event"`, `"task"`, `"repo"`, `"rev"`,
`"claim"`, `"evidence"`, `"suggested_next_run"`, `"cost_usd"`,
`"artifact_paths": ["REPORT.json"]`, and `"residual_risk"`.

The task writes `REPORT.json` only. It does not edit code, comment, merge,
deploy, park tasks, resolve runs, replay dead letters, or run a builder.

