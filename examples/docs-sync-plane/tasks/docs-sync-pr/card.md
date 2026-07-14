# Docs sync PR writer

## Goal

Consume an existing, actionable `docs-sync` `REPORT.json` (never investigate
drift from scratch) and open exactly one bounded pull request that applies its
recommended docs changes.

## Oracle

The writer reads the prior report named in the manual payload, confirms it has
at least one `drift_findings` entry, and opens a PR touching only the files
named in that report's `recommended_changes`. If the source report found no
drift, the writer opens no PR and says so directly.

## Boundaries

A refused credential is a boundary, not a puzzle: on HTTP 401/403 (or any
authorization refusal) from a credential this run declares, STOP-and-report —
write `REPORT.json` naming the refused operation and the refused credential by
name (never its value), then stop without completing the goal. Never locate or
use a stronger credential (env, keychain, 1Password, config, another agent).

PR-only. This task may: read the source `docs-sync` report, clone/checkout
only the repo declared in this task's `workspace.repos` (currently
`example-org/product-api`, a narrower allowlist than the report-only
watcher's two repos), create one branch, edit only the doc files the source
report named, and open exactly one pull request.

This task must never: merge, deploy, edit any file outside the source
report's `recommended_changes` targets, touch a repo outside its allowlist,
or open a second active pull request for the same source report. Before
branching, the agent must check for an existing open PR against the
allowlisted repo from a prior run of this task (`gh pr list --repo <repo>
--state open --search "docs-sync"` or equivalent) and refuse to open a
duplicate if one is found -- this check is agent-verified prose, the same
duplicate-suppression contract `canary-remediate` (backlog 115) already
follows, not a plane-enforced mechanism, because no webhook trigger backs
this manual-only task with a ledger-level dedupe key.

Manual dispatch only -- this task declares no cron or webhook trigger, and
`docs-sync-writer`'s own `policy.trigger_bindings` declares only `manual`,
matching `canary-remediate`'s precedent.

## Output

Write `REPORT.json` using the shape in `samples/REPORT-pr.json` (schema
`bb.docs_sync_pr.report.v1`): `schema`, `repo`, `source_report` (the run id of
the `docs-sync` report this PR is based on), `duplicate_check` (whether an
existing open PR was found and how it was checked), `pr` (whether one was
opened, its number, URL, and branch), `changed_files`,
`forbidden_actions_confirmed`, `artifacts`, `cost_usd`, and `residual_risk`.

## Receipt

The final answer repeats the source report consulted, whether a PR was opened
or refused as a duplicate, the PR URL when one exists, and the path to
`REPORT.json`.
