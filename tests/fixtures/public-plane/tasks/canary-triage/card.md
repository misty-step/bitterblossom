# Canary triage report-only commission fixture

## Goal

Triage one Canary incident as a report_only workflow. Read `RUN.json` first,
then `EVENT.json`, query Canary replay/read URLs when credentials are present,
and write a bounded incident triage report. This task never mutates code,
Canary, GitHub, deploy state, BB task state, or BB run state.

The declared service mapping includes both `canary` and `linejam`; Linejam
Production Smoke incidents are report-only evidence and never grant this task
code or deployment authority.

Read RUN.json first. Read EVENT.json next.

## Oracle

The report contains `"canary_subject"`, `"delivery_id"`, `"bb_run_id"`,
`"service"`, `"repo"`, `"evidence"`, `"hypotheses"`,
`"recommended_actions"`, `"constraints"`, and `"residual_uncertainty"`.
Every report links the incident id, service, environment, severity,
fingerprint, replay URLs, suspected files/services when known, and confidence.

## Boundaries

This task is `report_only`.

No code edits. No branches. No PRs. No merges. No deploys. Do not create
remediation claims. Do not annotate, acknowledge, close, resolve, or update
Canary incidents. Do not park or unpark tasks. Do not resolve BB runs. Do not
send user-visible notifications.

The agent may recommend exact next commands and owner files, but must not run
the commands. Canary access is read-only: replay URLs, incident detail, report,
timeline, and logs/traces may be read when credentials are present.

## Output

Write `REPORT.json` using schema `bb.canary_incident_response.report.v1` with:

- `schema`
- `canary_subject`
- `delivery_id`
- `bb_run_id`
- `service`
- `repo`
- `evidence`
- `hypotheses`
- `recommended_actions`
- `constraints.report_only`
- `constraints.mutations_performed`
- `residual_uncertainty`

`constraints.mutations_performed` must be an empty array.

## Receipt

The final answer repeats the incident id, service, top hypothesis, recommended
next action, and the path to `REPORT.json`. Name every Canary URL or artifact
inspected.
