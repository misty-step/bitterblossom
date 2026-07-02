# Canary triage commission fixture

This is a report_only incident triage card. No code edits. No branches. No PRs.
No deploys. Read RUN.json first. Read EVENT.json next. You must query Canary before
reasoning, create or observe a remediation claim, then write `REPORT.json`.

The report contains `"canary_subject"`, `"delivery_id"`, `"bb_run_id"`,
`"service"`, `"repo"`, `"evidence"`, `"hypotheses"`, and
`"residual_uncertainty"`.

The agent may recommend exact next commands but must not run them.
