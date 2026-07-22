# Bitterblossom Dogfood Notes Template

Use this shape for `docs/plans/YYYY-MM-DD-<topic>-dogfood.md`.

## Context

- Goal:
- Backlog item:
- `bb` binary:
- Plane:
- Sprite org:
- Sprite:
- Build run:
- PR:
- Commit/submission:

## Preflight

- `git status`:
- `scripts/production-ops-drill.sh --primary` (live HTTP/store readback and BLOCKED/PASS):
- `sprite org list` before/after:
- launchd label/program/workdir readback:
- `bb task list --json` summary:
- `bb runs list --json` summary:
- `bb dlq list --json` summary:

## Work

- `bb run build`:
- Build `REPORT.json`:
- Branch checkout:
- PR:
- Local verification:
- `bb submit open`:
- `bb run` members:
- `bb gate`:

## UX Notes

### Good

- Observation:
- Evidence:
- Lean in:

### Bad

- Observation:
- Evidence:
- Mitigate:

### Ugly

- Observation:
- Evidence:
- Mitigate:

### Friction

- Observation:
- Evidence:
- Mitigate:

### Bugs

- Observation:
- Evidence:
- Mitigate:

### Delight

- Observation:
- Evidence:
- Lean in:

## Reflection

- Does it work?:
- Does it produce useful results?:
- Does it feel good?:
- Too complicated / awkward?:
- Errors or unclear communication?:
- More steps than necessary?:
- Fits project vision?:
- Backlog-worthy improvements:
- No action:

## Backlog Emissions

- Added:
- Updated:
- Proposed:

## Closeout

- Final git status:
- Remote sync:
- Remaining parked tasks:
- Remaining DLQ:
- Next best pickup:
