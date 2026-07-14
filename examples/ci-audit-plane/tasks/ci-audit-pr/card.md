# CI gate hardener

## Goal

Consume an existing, actionable `ci-audit` `REPORT.json` (never audit a
repo's gates from scratch) and open exactly one bounded pull request that
adds or strengthens the checks that report proposed.

## Oracle

The hardener reads the source report named in the manual payload, confirms
it has at least one `missing_or_weak_gates` entry, and opens a PR that
implements entries from that report's `proposed_checks`. If the source
report found no gap, the hardener opens no PR and says so directly. Every
change in the PR must add or tighten a check; the hardener refuses to open a
PR that would remove, skip, loosen a threshold on, or otherwise weaken any
existing gate, test, or lint -- this is the one absolute red line for this
task family and it applies even if the source report's own
`proposed_checks` somehow asked for a weakening (which would itself be a bug
in the source report worth flagging in `residual_risk`, not something to
act on).

## Boundaries

A refused credential is a boundary, not a puzzle: on HTTP 401/403 (or any
authorization refusal) from a credential this run declares, STOP-and-report —
write `REPORT.json` naming the refused operation and the refused credential by
name (never its value), then stop without completing the goal. Never locate or
use a stronger credential (env, keychain, 1Password, config, another agent).

PR-only. This task may: read the source `ci-audit` report, clone/checkout
only the repo declared in this task's `workspace.repos` (currently
`example-org/product-api`, a narrower allowlist than the auditor's two
repos), create one branch, add or strengthen only the checks the source
report named, and open exactly one pull request.

This task must never: merge, deploy, weaken, loosen, skip, or remove any
existing gate/test/lint, edit any file outside what the hardening requires,
touch a repo outside its allowlist, or open a second active pull request for
the same source report. Before branching, the agent must check for an
existing open PR against the allowlisted repo from a prior run of this task
(`gh pr list --repo <repo> --state open --search "ci-audit"` or equivalent)
and refuse to open a duplicate if one is found -- this duplicate-active-work
refusal is agent-verified prose, the same contract `canary-remediate`
(backlog 115) and `docs-sync-pr` (backlog 120) already follow, not a
plane-enforced mechanism, because no webhook trigger backs this manual-only
task with a ledger-level dedupe key.

Manual dispatch only -- this task declares no cron or webhook trigger, and
`ci-hardener`'s own `policy.trigger_bindings` declares only `manual`,
matching `canary-remediate`'s and `docs-sync-pr`'s precedent.

## Output

Write `REPORT.json` using the shape in `samples/REPORT-pr.json` (schema
`bb.ci_audit_pr.report.v1`): `schema`, `repo`, `source_report` (the run id of
the `ci-audit` report this PR is based on), `duplicate_check`, `pr`,
`gates_added`, `gates_weakened` (must always be an empty array -- a non-empty
value here is an immediate hold condition, never a valid outcome),
`artifacts`, `cost_usd`, and `residual_risk`.

## Receipt

The final answer repeats the source report consulted, whether a PR was
opened or refused as a duplicate, the PR URL when one exists, confirms
`gates_weakened` is empty, and names the path to `REPORT.json`.
