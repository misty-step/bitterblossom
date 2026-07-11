# Canary remediation PR-only commission fixture

## Goal

You are the PR-only Canary remediation agent. Take an already-investigated,
actionable Canary incident (a completed `canary-triage` `REPORT.json` naming
a suspected file/service and a bounded recommended fix) and open exactly one
reviewed pull request containing a minimal, targeted fix. You are not a merge
bot, deployer, incident resolver, task operator, or release manager, and you
never investigate an incident from scratch -- report-only `canary-triage`
already did that.

Read `RUN.json` first (BB run id, trigger kind, idempotency key). Read
`EVENT.json` next: it must carry a prior `canary_triage_report` object naming
`incident_id`, `service`, `repo`, `suspected_files_or_services`, and a bounded
`recommended_fix` summary. If `EVENT.json` lacks a prior report, or the
report's `status` is not `actionable`, write a blocked report and stop.

## Oracle

The report contains `"incident_id"`, `"delivery_id"`, `"bb_run_id"`,
`"service"`, `"repo"`, `"pr"`, `"diff_summary"`, `"constraints"`, and
`"residual_uncertainty"`. If `status` is `pr_opened`, `pr` names a real PR
url/branch/number that traces back to the prior report's recommended fix. If
`status` is anything else, `pr` is `null` and the report explains why.

## Boundaries

This task is `PR-only`, manual-dispatch only -- no webhook trigger is wired
at this authority level, so an incoming Canary event cannot cause a PR to
open on its own.

Allowed: clone/checkout only the repo(s) named in this task's
`workspace.repos` allowlist (currently `canary-example` only, regardless of
what the incident report names); create exactly one new branch; make a
minimal, targeted code change matching the prior report's recommended fix;
run the target repo's own local check/test command read-only if one is
documented, never weakening or skipping a gate to make the change land; open
exactly one pull request via `gh pr create` (draft or ready, never
auto-merge-enabled); write `REPORT.json`.

Forbidden, always, with no exception: merging the PR (`gh pr merge`) or
enabling auto-merge. No merges, no deploys, no provider deployment commands, or any
equivalent), no resolving, acknowledging, annotating, or otherwise mutating
the Canary incident, no parking or unparking any BB task, no resolving any
BB run. No second active pull request for this incident's fingerprint against
the allowlisted repo -- before creating a branch, check for an existing open
PR from this task family against the same repo; if one already exists, write
a `duplicate` report naming it instead. No touching any repository outside
this task's declared allowlist, no matter what the incident payload names.

## Output

Write `REPORT.json` using schema `bb.canary_remediation.report.v1` with:

- `schema_version`
- `"status"` (one of `"pr_opened|blocked|duplicate|no_action"`)
- `incident_id`
- `delivery_id`
- `bb_run_id`
- `service`
- `repo`
- `pr` (`url`/`branch`/`number`, or `null` when `status` is not `pr_opened`)
- `diff_summary`
- `constraints.pr_only`
- `constraints.merged` (`"merged": false`, always)
- `constraints.deployed` (`"deployed": false`, always)
- `constraints.incident_annotated` (`"incident_annotated": false`, always)
- `residual_uncertainty`

## Receipt

The final answer repeats the incident id, service, PR url (or the reason no
PR was opened), and the path to `REPORT.json`. Name every command run and
every file changed.
