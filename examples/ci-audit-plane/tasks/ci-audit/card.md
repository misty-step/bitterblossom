# CI gate auditor

## Goal

Proactively audit one explicitly allowlisted repo's own CI: its gates, tests,
and lints as they exist today, not as a reaction to a specific failure. This
is distinct from the `ci-diagnose` reflex, which investigates one already-
failed CI signal named in an incoming webhook; this task looks for gates that
are missing, weak, or absent entirely, whether or not anything just failed.

## Oracle

The auditor inspects the repo's own CI configuration (workflow files, test
runners, lint configs, the repo's documented gate entrypoint) and writes
`REPORT.json` naming what currently gates merges, what is missing or weak,
and exact reproduction commands a human could run to see the same evidence.
It never proposes weakening an existing check.

## Boundaries

A refused credential is a boundary, not a puzzle: on HTTP 401/403 (or any
authorization refusal) from a credential this run declares, STOP-and-report —
write `REPORT.json` naming the refused operation and the refused credential by
name (never its value), then stop without completing the goal. Never locate or
use a stronger credential (env, keychain, 1Password, config, another agent).

Report only. Do not edit files, push branches, open PRs, weaken an existing
gate, merge, deploy, or post comments. Findings may include exact diffs or
config snippets as guidance, but no mutation happens in this template.

Manual dispatch requires a payload naming the repo to audit:

```json
{"repo": "example-org/product-api"}
```

`repo` must exactly match one entry in this task's `workspace.repos`
allowlist (currently `example-org/product-api` and
`example-org/bitterblossom-example`); a payload naming any other repo is
refused before the audit runs, and `REPORT.json` is not written for a
refused payload. The cron trigger audits every allowlisted repo in turn on
its daily schedule.

## Output

Write `REPORT.json` using the shape in `samples/REPORT.json` (schema
`bb.ci_audit.report.v1`): `schema`, `repo`, `trigger` (kind plus the
manual/cron reference that caused this run), `current_gates` (what already
gates a merge, and how it is invoked), `missing_or_weak_gates`,
`proposed_checks` (never a weakening of an existing gate), `risk`,
`reproduction_commands` (exact commands to see the same evidence locally),
`artifacts`, `cost_usd`, and `residual_risk`.

## Receipt

The final answer repeats the repo audited, the trigger, whether any gate is
missing or weak, the single most important proposed check, and the path to
`REPORT.json`.
