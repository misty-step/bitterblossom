# Canary incident response

## Goal

Triage one Canary incident from `EVENT.json` or a manual payload. Produce a
report-only response that helps the operator decide whether to page a human,
open a remediation issue, or keep watching.

## Oracle

The responder reads `RUN.json` first, reads `EVENT.json` next, queries Canary
using the replay URLs when credentials are present, and writes `REPORT.json`
with incident identity, evidence, hypotheses, recommended actions, and residual
uncertainty.

## Boundaries

Report only. Do not edit code, create branches, open PRs, deploy, acknowledge
alerts, close incidents, or mutate Canary state. Recommendations may include
exact commands or owners, but execution stays with the operator.

## Output

Write `REPORT.json` using the shape in `samples/REPORT.json`: schema,
canary_subject, delivery_id, bb_run_id, service, repo, evidence, hypotheses,
recommended_actions, and residual_uncertainty.

## Receipt

The final answer repeats the incident id, service, top hypothesis, recommended
next action, and the path to `REPORT.json`.
