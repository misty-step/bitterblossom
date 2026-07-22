# Linejam production-smoke operator alert

## Goal

Handle only Canary incidents for the `linejam` service carrying the exact
`linejam-production-smoke` monitor signal. The command wrapper creates one
Powder `request_input` item after the declared failure threshold and stops
without model execution, branches, pull requests, merges, or deploys. A
correlated `incident.resolved` delivery answers and completes that owned Powder
alert after recovery.

Canary sends `incident.opened`, `incident.updated`, and `incident.resolved`
through one service-scoped Linejam subscription to the `linejam-alert` route.
Do not create a separate recovery subscription. Generic incident remediation
uses the `incident-triage` route and never accepts Linejam or resolved events.
Only an actual `incident.resolved` delivery with its event-correlated success
annotation may answer or complete a Powder alert. A success annotation arriving
with `incident.opened` or `incident.updated` is recorded but never closes work.

## Single-flight contract

Every Linejam alert run uses the dedicated tailnet workspace host
`ssh://bb-runner@10.108.0.4:2222`. The non-default port avoids the hosted service's
managed SSH-port boundary while staying private to the peered VPC.
Bitterblossom holds that host's atomic lease for the full wrapper attempt.
Powder deliberately returns the same run to repeated
claims by this scoped actor, while `request_input` itself is not idempotent;
never invoke this wrapper outside the BB host lease. If an attempt stops after
claiming but before requesting input, the next leased attempt reclaims the same
run and finishes the operator handoff.

The task deliberately has no task-wide daily run cap: opened/updated churn must
never consume the admission needed by the eventual resolved delivery. Work
remains bounded by the 10-minute timeout, per-run cost and output caps, one-host
lease, delivery-id dedupe, and task-scoped attention-debt brake.

## Boundaries

A refused credential is a boundary, not a puzzle: on HTTP 401/403 (or any
authorization refusal) from a credential this run declares, STOP-and-report —
write `REPORT.json` naming the refused operation and the refused credential by
name (never its value), then stop without completing the goal. Never locate or
use a stronger credential (env, keychain, 1Password, config, another agent).

Read `RUN.json`, `EVENT.json`, and this card before acting. Reject any service
other than `linejam`, any monitor other than `linejam-production-smoke`, and
any event outside opened, updated, or resolved. Use credentials only through
environment variables and never print them. The alert path may read Canary and
create, request input on, answer, or complete its incident-keyed Powder card;
it has no code or production mutation authority.

## Oracle

`REPORT.json` uses `bb.incident_triage_response.v1`, names the Linejam incident,
the correlated GitHub Actions URL and failing spec when present, and reaches
one honest terminal status: below threshold, operator attention requested or
already owned, non-resolved success ignored, recovery annotated, recovery alert
closed, or blocked. It lists only `REPORT.json` in `artifact_paths` and contains
no secret material.

## Output

Write only `REPORT.json` as the durable artifact. Preserve the wrapper's
incident-keyed Powder card and run identifiers, bounded failure detail, and
correlated external URL. Never include raw HTTP credentials or webhook secret
values.

## Receipt

The command wrapper emits one final `bb.command_result.v1` JSON object on
stdout naming the terminal alert outcome. `REPORT.json` is the authoritative
receipt and its `artifact_paths` must equal `["REPORT.json"]`.
