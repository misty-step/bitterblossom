# Docs sync watcher

## Goal

Inspect the repository change named in `EVENT.json` or the manual payload and
identify docs, runbook, or operator-contract drift. Produce a report-only
sync recommendation for the maintainer.

## Oracle

The watcher compares changed source surfaces against likely documentation
targets and writes `REPORT.json`. It names concrete drift findings when found;
if no docs action is needed, it records the evidence and says so directly.

## Boundaries

Report only. Do not edit files, push branches, open PRs, create issues, change
labels, or post comments. Recommendations may include exact file paths and
patch guidance, but mutations stay outside this template.

## Output

Write `REPORT.json` using the shape in `samples/REPORT.json` (schema
`bb.docs_sync.report.v2`): `schema`, `repo`, `trigger` (kind plus the
manual/cron/webhook reference that caused this run: `source_ref`,
`source_sha`, and `delivery_id` when triggered by a webhook), `changed_files`
(the source files that motivated this check), `docs_targets`,
`drift_findings`, `recommended_changes`, `skipped_mutations`, `artifacts`
(released artifact paths, always includes `REPORT.json`), `cost_usd`, and
`residual_risk`.

## Receipt

The final answer repeats the repo/ref inspected, whether drift was found, the
top recommended change, and the path to `REPORT.json`.
