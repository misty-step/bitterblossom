# 064 Secret Preflight and DLQ Acknowledgement Dogfood

## Context

- Goal: deliver backlog 064 end to end with `bb` as the authoring surface, then
  review, live-test, and squash-merge the item PR to `master`.
- Backlog item: `backlog.d/064-secret-preflight-and-dlq-acknowledgement.md`
- `bb` binary: `./target/debug/bb`
- Plane: `plane`
- Sprite org: `misty-step`
- Sprite: `lane-1`
- Build run: `6637761a726b`
- PR: pending
- Commit/submission: builder commit `7ca1839266e5813e92626249b2ea65faad29a14b`;
  local review patch and backlog archive pending commit.

## Preflight

- `git status`: `master` clean and synced with `origin/master` before dispatch.
- `flyctl orgs list`: `Misty Step` / `misty-step`.
- `sprite org list`: selected `misty-step`.
- `sprite use -o misty-step lane-1`: succeeded.
- `sprite exec -- whoami`: `sprite`.
- `bb check`: passed for 29 plane tasks.
- `bb status --json` summary before dispatch: `cost_today_usd =
  0.1082236292`, `max_cost_per_day_usd = 25.0`, `open_dlq = 10`,
  `parked_tasks = 0`, `tasks = 29`.
- `bb task list --json` summary after checkout: 29 tasks, no parked tasks;
  `build` uses `bb-builder-rust@v2`, `omp`, `z-ai/glm-5.2`, `sprites`.
- `bb runs list --json` summary: latest build became run `6637761a726b`.
- `bb dlq list --json` summary after the branch migration code ran locally:
  12 rows total, 10 `open`, 2 `replayed`.

## Work

- `bb run build`: `GH_TOKEN=$(env -u GITHUB_TOKEN gh auth token)
  ./target/debug/bb --config plane run build --payload
  '{"repo":"misty-step/bitterblossom","backlog":"backlog.d/064-secret-preflight-and-dlq-acknowledgement.md","branch_slug":"064-secret-preflight-dlq-ack","dry_run":false}'
  --json`
- Build result: `success`, `$3.283245500000001`, `1475198ms`,
  `419704/37139` tokens, 121 turns, artifact dir
  `plane/.bb/runs/6637761a726b/attempt-1`.
- Build `REPORT.json`: status `ready`; branch
  `bb/build/064-secret-preflight-dlq-ack`; verification claimed
  `./scripts/verify.sh` passed with 138 tests and `src LOC 5411`.
- Branch checkout: `git checkout bb/build/064-secret-preflight-dlq-ack`.
- Local verification:
  - `cargo test --test preflight_cli --test dlq_ack_cli -- --nocapture` passed
    on the builder commit.
  - Local review found the replay/acknowledge mutual exclusion was enforced by
    caller checks but not fully by ledger update predicates. Fixed in
    `src/ledger.rs` and added
    `dead_letter_replay_and_acknowledge_are_mutually_exclusive_in_ledger`.
  - `cargo fmt --all && cargo test --test ledger
    dead_letter_replay_and_acknowledge_are_mutually_exclusive_in_ledger
    -- --nocapture && cargo test --test dlq_ack_cli --test preflight_cli
    -- --nocapture` passed.
  - `./scripts/verify.sh` passed after the local review fix with
    `src LOC: 5421`.
- Live preflight proof:
  - `./target/debug/bb --config plane preflight --storm --json` exited `2`
    and named missing `GH_TOKEN` for `verify`, `correctness`, `security`,
    `simplification`, and `product`.
  - `GH_TOKEN=$(env -u GITHUB_TOKEN gh auth token) ./target/debug/bb --config
    plane preflight --storm --json` exited `0` with no findings.
- Live DLQ acknowledgement proof on a disposable temp plane:
  - `bb run broken --json` created a pre-execute DLQ.
  - `bb dlq ack 1 --reason 'superseded by replacement submission' --json`
    returned `status = "acknowledged"` with `acknowledged_reason` and
    `acknowledged_at`.
  - `bb status --json` reported `summary.open_dlq = 0` and
    `broken.dlq.acknowledged = 1`.
  - `bb dlq replay 1 --json` exited `1` with `replay rejected`.
- `bb submit open`: pending.
- `bb run` members: pending.
- `bb gate`: pending.

## UX Notes

### Good

- Observation: the build run produced a complete implementation with tests,
  docs, schema migration, command surfaces, and a structured `REPORT.json`.
- Evidence: `REPORT.json` named branch, commit, verification, summary, and
  residual risk.
- Lean in: keep requiring `REPORT.json`; it made review fast.

### Bad

- Observation: `bb run --json` stayed silent for 24.6 minutes. The ledger only
  showed `executing` and stale `updated_at`.
- Evidence: run `6637761a726b` had only `EVENT.json`, `LANE_CARD.md`, and
  `RUN.json` until final collection.
- Mitigate: likely covered by observability backlog 072; add remote heartbeat
  or attempt progress to a future slice instead of expanding 064.

### Ugly

- Observation: the run reported 121 turns even though `build` has
  `turn_cap = 80`.
- Evidence: `bb runs show 6637761a726b --json` attempt fields:
  `turns = 121`; task config has `turn_cap = 80`.
- Mitigate: treat as a product bug if repeated; investigate OMP turn-cap
  enforcement separately, because 064 should not absorb harness-limit work.

### Friction

- Observation: `bb status --json` reported 10 open DLQs while `bb dlq list
  --json` had 12 rows; after the branch, the distinction is clearer because
  rows expose `status`.
- Evidence: `dlq list --json` grouped 10 `open` and 2 `replayed`.
- Mitigate: no new backlog; 064 directly improves this scan path.

### Bugs

- Observation: builder implementation left replay/ack mutual exclusion partially
  outside the ledger.
- Evidence: `mark_dead_letter_replayed` and `acknowledge_dead_letter` update
  predicates did not both require the row to still be open.
- Mitigate: fixed locally with a ledger-level test before PR.

### Delight

- Observation: `bb preflight --storm --json` is a compact command for the exact
  missing-secret storm failure that created the ticket.
- Evidence: new focused tests cover missing secrets, unspawnable local command
  binaries, and storm member preflight.
- Lean in: preflight should stay read-only and cheap.

## Reflection

- Does it work?: yes, for the local proof surface. The branch adds a read-only
  preflight command and an explicit DLQ acknowledgement path with tests and
  full gate proof.
- Does it produce useful results?: yes. The PR is narrow enough to review:
  `src/preflight.rs`, DLQ status plumbing, docs, and focused tests.
- Does it feel good?: partly. The final report is useful; the long silent
  execution does not feel good.
- Too complicated / awkward?: the new operator commands are straightforward.
  The branch is bigger than an S slice, but the size is mostly tests and docs.
- Errors or unclear communication?: no build failure; communication gap is
  progress visibility while executing.
- More steps than necessary?: still many manual join steps after build
  (fetch, checkout, inspect report, run local gate, open PR, submission storm).
- Fits project vision?: yes. This is event-plane operator state: declared
  secrets, command availability, DLQ state, status truth, and immutable replay
  history. It does not move workload judgment into Rust.
- Backlog-worthy improvements: no new item from 064; progress visibility is
  already covered by 072, and turn-cap enforcement should be investigated if it
  repeats.
- No action: no new backlog for the local ledger review fix because it was
  fixed in this branch.

## Backlog Emissions

- Added: none.
- Updated: moved `backlog.d/064-secret-preflight-and-dlq-acknowledgement.md`
  to `backlog.d/_done/064-secret-preflight-and-dlq-acknowledgement.md`.
- Proposed: none.

## Closeout

- Final git status: pending.
- Remote sync: pending.
- Remaining parked tasks: 0 before dispatch and after local status checks.
- Remaining DLQ: 10 open, 2 replayed on the live plane after branch migration
  opened the local DB.
- Next best pickup: after 064 lands, choose among P1 ready `051`, `072`, or
  stale-check `073`.
