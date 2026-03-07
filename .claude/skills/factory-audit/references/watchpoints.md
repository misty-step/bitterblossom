# Watchpoints

Use this checklist while a supervised run is live.

## Preflight

- `bb` builds cleanly
- auth resolves from `.env.bb`
- at least one worker is reachable
- chosen worker is already prepared for the repo

## Builder phase

- `lease_acquired` and `builder_selected` appear quickly
- builder opens a PR and writes `builder-result.json`
- conductor records `builder_complete` without long blind gaps
- worker workspace is not polluted by stale run state

## Council phase

- each reviewer writes an artifact
- `review_complete` events appear as artifacts land
- conductor comments the council verdict coherently
- reviewer workspaces are clean enough that output is attributable to this run

## PR governance phase

- PR transitions from draft to ready at the intended point
- required CI reruns after ready-for-review
- external review surfaces are observed, not ignored
- unresolved threads, pending statuses, and quiet-window semantics are explicit

## Merge phase

- merge occurs only after policy is truly settled
- GitHub timestamps align with run-store timestamps
- no pending trusted review surface remains at merge time

## Post-run

- issue, PR, and run ledger agree on terminal state
- local operator path to understand what happened is short
- friction converts into backlog updates, not hand-wavy notes
