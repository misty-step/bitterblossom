# 2026-06-18 070 fix-prompt dogfood

## Context

- Goal: Use `bb` itself to deliver backlog 070, then critique the build,
  submission, gate, ledger, and operator UX.
- Backlog item: `backlog.d/070-gate-blocked-fix-prompt-reflex.md`
- `bb` binary: `./target/debug/bb`
- Plane: `plane`
- Sprite org: started on `adminifi`, switched to `misty-step`
- Sprite: `lane-1`
- Build run: `380ca26ed25b` (`failure`)
- PR: pending
- Commit/submission: base `337f4303de99e9c051f511f4b22e5dc65239fb82`

## Preflight

- `git status`: `## master...origin/master`
- `flyctl orgs list`: `misty-step` available
- `sprite org list` before/after: before `adminifi`; after `misty-step`
- `bb check`: passed, 28 plane tasks loaded
- `bb task list --json` summary: 28 tasks, build is unparked, no runs today
- `bb runs list --json` summary: recent storm runs from 2026-06-16; no active
  run found in preflight output
- `bb dlq list --json` summary: 10 open DLQs, mostly stale `GH_TOKEN` missing
  failures from prior submission storms

## Work

- `bb run build`: failed run `380ca26ed25b` after 17.6s
- Build `REPORT.json`: none; the harness failed before producing a report
- Branch checkout: skipped; no branch was pushed
- PR: skipped; no branch was pushed
- Local verification: `./scripts/verify.sh` passed after adding dogfood notes
  and backlog 073
- `bb submit open`: skipped; no PR/commit to submit
- `bb run` members: skipped
- `bb gate`: skipped

## UX Notes

### Good

- Observation: `bb status --json` gives enough task-level summary to see build
  budget, queue state, prior failure, and safe next action in one query.
- Evidence: preflight `jq` summary included build latest failure and DLQ tasks.
- Lean in: keep status as the operator truth surface.

### Bad

- Observation: The dogfood preflight requires many separate commands before the
  first build dispatch.
- Evidence: `git status`, Fly orgs, Sprite orgs, Sprite switch, `bb check`,
  `bb status`, task list, runs list, and DLQ list are separate manual probes.
- Mitigate: consider a dogfood/preflight command or status bundle if this repeats.

### Ugly

- Observation: latest `build` failure is Codex subscription refresh on the
  sprite, but `bb status` can only suggest inspecting that old run, not proving
  whether the auth problem is still live before spending a new run.
- Evidence: build latest failure `cb01d8b0f17e` reports `refresh_token_reused`.
- Mitigate: added `backlog.d/073-dispatch-readiness-for-subscription-builders.md`
  after this run failed the same way.

### Friction

- Observation: Sprite selected org was wrong for this repo.
- Evidence: `sprite org list` initially showed `Currently selected org:
  adminifi`; `sprite use -o misty-step lane-1` fixed it.
- Mitigate: keep the explicit org preflight; consider whether `bb` can surface
  substrate org mismatch before dispatch.

### Bugs

- Observation: `bb run build` accepted a full authoring run even though the
  configured Codex subscription auth on the sprite was unusable.
- Evidence: run `380ca26ed25b`, attempt `295`, stderr
  `plane/.bb/runs/380ca26ed25b/attempt-1/stderr.txt`, repeated
  `refresh_token_reused` and `token_expired`.
- Mitigate: `backlog.d/073-dispatch-readiness-for-subscription-builders.md`.

### Delight

- Observation: `sprite use -o misty-step lane-1` made the org correction
  explicit and `sprite exec -- whoami` gave a cheap proof.
- Evidence: command returned `sprite`.
- Lean in: preserve this cheap preflight proof.

## Reflection

- Does it work?: no for the primary authoring path; `bb` created a durable run
  and artifacts, but no branch or PR.
- Does it produce useful results?: useful failure evidence, not useful product
  output.
- Does it feel good?: partially. Ledger/artifacts were clear after failure, but
  the failure was predictable from old status and still required spending a run.
- Too complicated / awkward?: preflight feels command-heavy; final judgment
  pending after a build run.
- Errors or unclear communication?: build auth health is unclear before dispatch;
  the remediation is buried in raw Codex stderr rather than a `bb` readiness
  surface.
- More steps than necessary?: yes for preflight; the operator has to stitch
  together Git, Fly, Sprite, status, task list, runs, and DLQ health manually.
- Fits project vision?: yes, this tests terminal dispatch, durable ledger,
  costs, artifacts, and gates.
- Backlog-worthy improvements:
  `backlog.d/073-dispatch-readiness-for-subscription-builders.md`
- No action: do not file a separate "many preflight commands" ticket yet; 073
  is the sharp blocker, and 064/072 already cover adjacent readiness/status work.

## Backlog Emissions

- Added: `backlog.d/073-dispatch-readiness-for-subscription-builders.md`
- Updated: none
- Proposed: fold repeated preflight-command friction into 064/072 if it recurs
  after auth readiness is fixed.

## Closeout

- Final git status: pending commit
- Remote sync: pending push
- Remaining parked tasks: preflight had `0`
- Remaining DLQ: preflight had `10` open
- Next best pickup: unblock builder readiness before retrying dogfood delivery
