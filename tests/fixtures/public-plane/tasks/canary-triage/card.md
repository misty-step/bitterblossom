# Canary triage commission fixture

## Goal

Triage one Canary incident as a report_only workflow. Read `RUN.json` first,
then `EVENT.json`, query Canary before reasoning, and create or observe a
remediation claim.

Read RUN.json first. Read EVENT.json next.

## Oracle

The report contains `"canary_subject"`, `"delivery_id"`, `"bb_run_id"`,
`"service"`, `"repo"`, `"evidence"`, `"hypotheses"`, and
`"residual_uncertainty"`.

## Boundaries

No code edits. No branches. No PRs. No deploys. The agent may recommend exact
next commands but must not run them.

## Output

Write `REPORT.json` with the incident evidence, hypotheses, and residual
uncertainty.

## Receipt

The final answer repeats the `REPORT.json` summary and names the Canary
artifact or URL inspected.
