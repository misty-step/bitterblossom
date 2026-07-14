# Dependabot triage hygiene reflex

## Goal

For each configured repository, list open Dependabot PRs, classify their update
size, CI state, age, and dependency risk, then write a triage receipt.

REPORT mode is the default. Merge-on-green mode is a future graduated posture,
not active unless explicitly configured.

## Input

Read `RUN.json` first, then `EVENT.json`. `EVENT.json` must contain:

- `"mode": "report"` for triage only, or `"mode": "merge_on_green"` after
  operator graduation.
- `"repos"`: an array of repo configs with `"repo"` (`owner/name`) and optional
  `"merge_on_green_enabled"`.

## Oracle

`REPORT.json` uses schema `bb.dependabot_triage_report.v1` and includes
`"mode"`, `"authority"`, `"summary"`, `"merge_candidates"`, one PR entry per
open Dependabot PR, and `"artifact_paths": ["REPORT.json"]`. Every PR has a
decision and reasons.

## Classification

Classify each PR as:

- patch, minor, major, or unknown;
- CI state: green, pending, red, or unknown;
- dependency scope: dev/CI deps only, runtime, or unknown;
- age in days.

## Boundaries

A refused credential is a boundary, not a puzzle: on HTTP 401/403 (or any
authorization refusal) from a credential this run declares, STOP-and-report —
write `REPORT.json` naming the refused operation and the refused credential by
name (never its value), then stop without completing the goal. Never locate or
use a stronger credential (env, keychain, 1Password, config, another agent).

Report-only triage writes `REPORT.json` and does not merge.

Graduated merge-on-green may merge only when all are true:

- `EVENT.json.mode` is `"merge_on_green"`;
- the repo config has `"merge_on_green_enabled": true`;
- `DEPENDABOT_TRIAGE_ENABLE_MERGE=1` is present;
- the PR is patch or minor;
- the PR is a dev/CI deps only update;
- CI is green.

Conservative floor: never majors, never runtime deps, never unknown dependency
scope, and never red/pending/unknown CI without a human.

## Output

Write `REPORT.json` using schema `bb.dependabot_triage_report.v1` with:

- `"mode"`, `"authority"`, and `"summary"`;
- one entry per repo with `"open_dependabot_pr_count"`,
  `"merge_candidates_count"`, `"merged"`, and `"prs"`;
- each PR entry includes version class, dependency scope, CI state, age,
  decision, reasons, and `"would_merge"`;
- `"merge_candidates"` in the summary;
- `"artifact_paths": ["REPORT.json"]`;
- `"residual_risk"`.

The final command result should summarize repo count and merge candidate count.
No secrets in stdout, stderr, `REPORT.json`, or command arguments.

## Receipt

The final answer repeats the repo count, open Dependabot PR count, merge
candidate count, each PR decision, and whether merge mode was enabled.
