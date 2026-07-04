# Branch-prune hygiene reflex

## Goal

For each configured repository, list remote branches that are fully merged into
the default branch and produce a cleanup receipt. REPORT mode is the default and
is the only mode approved for this card today.

## Input

Read `RUN.json` first, then `EVENT.json`. `EVENT.json` must contain:

- `"mode": "report"` for dry-run reporting, or `"mode": "delete"` only after
  operator graduation.
- `"repos"`: an array of repo configs with `"repo"` (`owner/name`),
  optional `"default_branch"`, optional `"never"` glob patterns, and optional
  `"delete_enabled"`.

## Oracle

`REPORT.json` uses schema `bb.branch_prune_report.v1` and includes `"mode"`,
`"authority"`, `"summary"`, `"would_delete"`, `"kept"`, and
`"artifact_paths": ["REPORT.json"]`. Every kept branch names why it was never
eligible: `default_branch`, `unmerged`, `open_pr`, or `explicit_never`.

## Boundaries

DRY-RUN FIRST. In REPORT mode, write `REPORT.json` only. Do not mutate GitHub.

Deletion is graduated only when all are true:

- `EVENT.json.mode` is `"delete"`;
- the repo config has `"delete_enabled": true`;
- the environment contains `BRANCH_PRUNE_ENABLE_DELETE=1`.

If any flag is missing, keep reporting only. No automatic promotion.

## Never Delete

The NEVER list is load-bearing:

- the default branch;
- unmerged branches;
- any branch with an open PR;
- any branch matching explicit `never` patterns.

Never force-push. The only allowed deletion command in graduated mode is
`git push --delete` for a candidate that survived every NEVER check.

## Output

Write `REPORT.json` using schema `bb.branch_prune_report.v1` with:

- `"mode"`, `"authority"`, and `"summary"`;
- one entry per repo with `"default_branch"`, `"open_pr_branches"`,
  `"never"`, `"would_delete"`, `"deleted"`, `"delete_errors"`, and `"kept"`;
- every kept branch must name reasons such as `default_branch`, `unmerged`,
  `open_pr`, or `explicit_never`;
- `"artifact_paths": ["REPORT.json"]`;
- `"residual_risk"`.

The final command result should summarize repo count and branch count. No
secrets in stdout, stderr, `REPORT.json`, or command arguments.

## Receipt

The final answer repeats the repo count, total branch count, each repo's
`would_delete` list, and whether deletion mode was enabled.
