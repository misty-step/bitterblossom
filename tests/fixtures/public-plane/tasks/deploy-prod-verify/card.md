# Deploy and production verifier commission fixture

## Goal

Verify one deploy-smoke failure or production incident as a report_only
workflow. Read `RUN.json` first, read `EVENT.json` next, then collect concrete
browser/API evidence for the target in the event payload. Produce a suggested
next run command, not a mutation.

## Oracle

The report contains `"schema_version"`, `"event"`, `"bb_run_id"`,
`"service"`, `"environment"`, `"repo"`, `"revision"`, `"target_urls"`,
`"api_evidence"`, `"browser_evidence"`, `"claim"`, `"suggested_next_run"`,
`"artifact_paths"`, `"cost_usd"`, and `"residual_risk"`.

The report distinguishes deploy-smoke failure from production incident intake,
and every claim names the exact URL, method, status code, screenshot path, log
excerpt, or response excerpt inspected.

## Boundaries

A refused credential is a boundary, not a puzzle: on HTTP 401/403 (or any
authorization refusal) from a credential this run declares, STOP-and-report —
write `REPORT.json` naming the refused operation and the refused credential by
name (never its value), then stop without completing the goal. Never locate or
use a stronger credential (env, keychain, 1Password, config, another agent).

report_only. No code edits. No branches. No PRs. No deploys. Do not touch the
production ledger directly. Do not resolve runs, park tasks, unpark tasks,
create incidents, acknowledge incidents, or send user-visible notifications.
Use read-only browser/API checks only.

## Output

Write `REPORT.json` with a deploy/prod verification packet. Include the exact
follow-up `bb run ... --payload-file ...` command only as
`"suggested_next_run"` when a fix, rollback, or deeper diagnosis should happen.

## Receipt

The final answer repeats the `REPORT.json` summary, the concrete browser/API
evidence inspected, and the suggested next run if any.
