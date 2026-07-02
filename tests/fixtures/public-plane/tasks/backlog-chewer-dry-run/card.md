# Backlog chewer dry-run commission fixture

## Goal

Scan only whitelisted repositories and select at most one ready backlog ticket
for a future implementation run. This task is `dry_run`: it writes a plan and
classification report, not a branch.

## Input

Read `RUN.json` first, then read `EVENT.json` when present. The payload may
name:

- `"mode": "dry_run"`;
- `"repo_whitelist"` entries with repo name, local path, default branch, and
  allowed task families;
- `"backlog_root"` such as `backlog.d/`;
- `"max_candidates"` and `"budget"`;
- optional `"active_prs"` or a command output artifact that lists existing
  BB-authored PR pressure.

Ignore repositories that are not whitelisted. If the payload is missing a
whitelist, classify the run as blocked and write `REPORT.json`; do not infer a
repo from the filesystem.

## Oracle

Select a ticket only when it has a clear Goal, executable Oracle, bounded scope,
allowed credentials and side effects, and an obvious verifier. Under-specified
tickets produce a `shaping_packet` instead of implementation. Blocked,
dangerous, or destructive tickets go to `skipped_tickets` with reasons.

under-specified tickets must never be selected for implementation.

`REPORT.json` uses schema `bb.backlog_chewer_dry_run_report.v1` and includes
`"mode": "dry_run"`, `"repo"`, `"selected_ticket"`, `"shaping_packets"`,
`"skipped_tickets"`, `"authority"`, `"budget"`, `"duplicate_pressure"`,
`"suggested_next_run"`, `"artifact_paths": ["REPORT.json"]`, and
`"residual_risk"`. The selected ticket entry must name `"id"`, `"path"`,
`"readiness"`, `"goal"`, `"oracle"`, `"verifier"`, `"branch_name"`,
`"expected_changed_paths"`, and `"stop_conditions"`.

## Boundaries

This task is dry-run only. It writes `REPORT.json` only.

- No code edits.
- No branches.
- No PRs.
- No merges.
- No deploys.
- No comments.
- No task parking or unparking.
- No run resolution.
- Do not execute the suggested implementation command.
- Do not select outside whitelisted repositories.
- Do not select a ticket when a duplicate active BB-authored PR already exists
  for the same repo/task family; report the duplicate pressure instead.

## Output

Write `REPORT.json` with the selected ready ticket or a blocked/no-selection
result, every rejected candidate reason, any shaping packet, the exact verifier,
budget, stop conditions, branch name, and the suggested next `bb run ...`
command. Do not create files outside the attempt artifact directory.

## Receipt

The final answer repeats the selected ticket id, the suggested next command,
the skipped ticket ids, and whether any duplicate active work blocked selection.
