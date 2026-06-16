# 054 Operability Dogfood Notes

## Context

- Goal: Deliver backlog 054 while dogfooding BB and synthesizing feedback into backlog state.
- Backlog item: `backlog.d/054-production-operability-drills.md`
- `bb` binary: `./target/debug/bb`
- Plane: `plane`
- Sprite org: `misty-step`
- Sprite: `lane-1`
- Commit/submission: BB-reviewed commit
  `36c399d062ddb23ae835e7597c37a0319bad0aa4`, submission
  `2278a57ab2d1`, PR #859. The final branch commit may differ by a notes-only
  amend.

## Preflight

- `git status`: `## deliver/054-operability-drills`
- `flyctl orgs list`: Misty Step available as slug `misty-step`.
- `sprite org list` before/after: selected org was already `misty-step`; `sprite use -o misty-step lane-1` confirmed it; `sprite exec -- whoami` returned `sprite`.
- `bb check`: passed for `plane`.
- `bb task list --json` summary: 28 tasks, no parked tasks.
- `bb runs list --json` summary: latest final-review storm for PR #858 had five successful member runs after a failed no-`GH_TOKEN` attempt.
- `bb dlq list --json` summary: 10 open DLQs, including five from missing `GH_TOKEN` during submission `3992c39cfbc5`.

## Work

- Local changes:
  - Added `docs/operations/README.md`.
  - Added `scripts/production-ops-drill.sh`.
  - Wired `scripts/production-ops-drill.sh --local` into `./scripts/verify.sh`.
  - Updated `docs/spine.md`, CI workflow comments, and BB operator recipes.
  - Added backlog follow-up `064-secret-preflight-and-dlq-acknowledgement`.
  - Archived 054 to `_done` with delivery notes.
- Local verification:
  - `./scripts/production-ops-drill.sh --local` passed.
  - `cargo test --test cli_contract_docs -- --nocapture` passed.
  - `./scripts/verify.sh` passed.
- `bb submit open`: `2278a57ab2d1` for `pr-054-operability-drills`.
- `bb run` members:
  - `verify`: `90b19df7076e`
  - `correctness`: `989888011225`
  - `security`: `ba766a974f31`
  - `simplification`: `47cc01b09d7d`
  - `product`: `e4fd69ca0a41`
- `bb gate`: `clear`, zero findings.

## UX Notes

### Friction

- Observation: A submission storm can fan out into multiple dead letters when `GH_TOKEN` is not exported.
- Evidence: DLQ ids 8-12 from submission `3992c39cfbc5` all failed with `secret env var 'GH_TOKEN' not set`.
- Mitigate: 054 should add a runbook preflight that checks all declared task secrets before fanout; a future CLI follow-up should make this first-class.

- Observation: `bb dlq list --json` shows replay state but there is no command to mark a superseded pre-execute DLQ as acknowledged.
- Evidence: Five no-`GH_TOKEN` DLQs are now superseded by clear submission `d26e1e51da69`, but remain open.
- Mitigate: Add a backlog follow-up for `bb dlq acknowledge|resolve` or status grouping of superseded DLQs.

- Observation: Existing operator recipes still used `curl -H "Authorization: Bearer $BB_API_TOKEN"`.
- Evidence: `rg` found the old pattern in `skills/bitterblossom/references/operator-recipes.md`.
- Mitigate: Updated the recipe and new ops script to pipe curl config through stdin for bearer calls.

### Bugs

- Observation: None confirmed in current code before edits; the DLQ issue is a missing operator affordance, not a wrong state transition.
- Evidence: `bb status --json` and `bb dlq list --json` truthfully expose the failures.
- Mitigate: Keep as backlog/UX work unless implementation reveals an incorrect ledger invariant.

### Delight

- Observation: `bb gate --submission <id> --json` gives a compact, durable ship decision with member costs and zero reliance on chat memory.
- Evidence: PR #858 final gate `d26e1e51da69` was clear with five pass verdicts and no findings.
- Lean in: Keep using gate receipts as the closeout source for delivery branches.

- Observation: The local operations drill is fast enough to keep in `./scripts/verify.sh`.
- Evidence: It completed a local server/API/restore loop in a few seconds during this run.
- Lean in: Prefer local deterministic drills for production runbook claims, then keep the truly live Fly checks as explicit operator evidence.

## Backlog Emissions

- Added: `backlog.d/064-secret-preflight-and-dlq-acknowledgement.md`.
- Updated: `backlog.d/_done/054-production-operability-drills.md` carries dogfood synthesis and delivery notes.
- Proposed: Future BB preflight should validate a whole submission storm member set before run rows are created.

## Closeout

- Final git status: clean at first PR creation; notes amended afterward.
- Remote sync: branch pushed to `origin/deliver/054-operability-drills`.
- Remaining parked tasks: none at preflight.
- Remaining DLQ: 10 open at preflight; five are known superseded no-`GH_TOKEN` entries from submission `3992c39cfbc5`.
- Next best pickup: 064 if the operator wants to eliminate the DLQ/preflight friction next.
