# Bitterblossom Dogfood Notes: 070 Fix-Prompt Reflex

## Context

- Goal: deliver backlog 070 by adding a report-only fix-prompt reflex for blocked gates.
- Backlog item: `backlog.d/070-gate-blocked-fix-prompt-reflex.md`
- `bb` binary: `./target/debug/bb`
- Plane: `plane`
- Sprite org: preflight started on `adminifi`, corrected to `misty-step`.
- Sprite: `lane-1`
- Build run: pending
- PR: pending
- Commit/submission: starting from `ad36ae18adcf7100ff30587c3a81f3dfae2f1cab`

## Preflight

- `git status`: `## master...origin/master`
- Remote sync: `0 0` for `master...origin/master`
- `flyctl orgs list`: `misty-step` available.
- `sprite org list` before/after: selected org was `adminifi`; after `sprite use -o misty-step lane-1`, selected org was `misty-step`.
- `sprite exec -- whoami`: `sprite`
- `bb check`: all 28 plane tasks loaded.
- `bb status --json` summary: cost today `$1.7613540263` / `$25.00`; 28 tasks; parked tasks `0`; open DLQ `10`.
- `bb task list --json` summary: `build` is OMP / `z-ai/glm-5.2`, not parked, cap `$4.00`, one run today.
- `bb runs list --json` summary: latest completed runs are the 075 gate storm and model-eval/build calibration runs.
- `bb dlq list --json` summary: 10 old open DLQ rows, mostly missing `GH_TOKEN` from earlier storm runs plus older substrate/preflight failures.

## Work

- `bb run build`: `49a964da550f`, success, `$0.8166981`, `170262/14837`
  tokens, 48 turns, `702231ms`.
- Build `REPORT.json`: status `ready`, branch
  `bb/build/070-fix-prompt-reflex`, original commit
  `791bc68b23eb94d9f452504809f8688b6460c017`.
- Branch checkout: fetched and checked out; branch was based on pre-075
  `master` and was rebased onto `ad36ae18adcf7100ff30587c3a81f3dfae2f1cab`.
- PR: pending
- Local verification: pending
- `bb submit open`: pending
- Live `fix-prompt` run: `b629d72e2c92`, success, `$0.0014680802`,
  `9786/2010` tokens, 4 turns, `30971ms`.
- Live `fix-prompt` artifact: `plane/.bb/runs/b629d72e2c92/attempt-1/REPORT.json`
  status `actionable`, packet fingerprint `de3c9ead006f1418`,
  file `scripts/check-model-catalog.sh`, line `109`, kind `security`, with a
  bounded `bb run build` recommendation.
- `bb run` members: pending for PR branch submission.
- `bb gate`: old blocked submission `21b1e4fb24a9` was read locally to build
  the live fix-prompt payload; no new settlement was needed because it was
  already `blocked`.

## UX Notes

### Good

- Observation: `bb status --json` made the current cost, parked task count, and task caps inspectable before dispatch.
- Evidence: preflight showed `parked_tasks = 0` and `build.max_cost_per_run_usd = 4.0`.
- Lean in: keep status as the operator's first truth surface.

### Bad

- Observation: Sprite org selection drifted back to `adminifi` even after prior correction.
- Evidence: `sprite org list` showed `Currently selected org: adminifi` before correction.
- Mitigate: keep the explicit org correction in the dogfood preflight until the plane can fail fast on the wrong org.
- Observation: The builder branch was based on the previous `master` even after
  the local checkout was synced to the current one.
- Evidence: branch head `791bc68` had merge base `97b93bc`; it needed a local
  rebase onto `ad36ae1` before verification.
- Mitigate: backlog-worthy if repeated: builder should refresh its target base
  at dispatch time or record the exact base it used as a warning.
- Observation: The builder-authored card tried to make the remote sprite call
  `bb gate`, but the sprite workspace does not own the local plane ledger.
- Evidence: `fix-prompt` task config had no repo/ledger materialization, yet
  card instructed `bb --config plane gate --submission "$submission" --json`.
- Mitigate: patched the contract so `gate.blocked` notify carries `rev` and the
  full `blocking` array; the fix-prompter consumes payload-only evidence and no
  longer receives `GH_TOKEN`.

### Ugly

- Observation: pending
- Evidence: pending
- Mitigate: pending

### Friction

- Observation: `bb runs list --json` is too large to be a comfortable preflight surface during long dogfood sessions.
- Evidence: the command emitted thousands of lines and was truncated in the terminal transcript.
- Mitigate: backlog-worthy if repeated: add a concise recent-run or health summary path for dogfood preflight.
- Observation: `bb run --json` is silent for the whole builder execution.
- Evidence: run `49a964da550f` produced no foreground output for about 12
  minutes; the ledger had to be polled separately for `executing` and branch
  side-effect state.
- Mitigate: backlog-worthy: JSON mode should optionally emit structured
  progress events or a first run-id envelope.
- Observation: The live fix-prompter wrote valid `REPORT.json` but still put
  a fenced JSON block into `result.md` despite the card's "No markdown fence"
  instruction.
- Evidence: `plane/.bb/runs/b629d72e2c92/attempt-1/result.md` wraps the same
  JSON in a fenced block; `REPORT.json` is correct.
- Mitigate: no action for this slice because the artifact contract passed; if
  downstream consumers read `result.md`, tighten/validate the human output.

### Bugs

- Observation: Builder output claimed the reflex card could re-fetch the gate
  report from the plane, which would fail or mutate from the wrong boundary.
- Evidence: the run report residual risk said the live reflex was untested; code
  review found the card's remote `bb gate` command before live execution.
- Mitigate: fixed before live dogfood; this is exactly the kind of workload
  judgment that must stay in card/payload contract rather than Rust.

### Delight

- Observation: The build task remains unparked after 075's OMP/GLM cap calibration.
- Evidence: task list shows `build` on OMP / GLM 5.2 with cap `$4.00` and no parked reason.
- Lean in: model/cost calibration now directly unlocks the self-dogfood loop.

## Reflection

- Does it work?: pending
- Does it produce useful results?: yes for the live manual path; report
  `b629d72e2c92` named the existing blocked fingerprint and produced a bounded
  build command.
- Does it feel good?: pending
- Too complicated / awkward?: pending
- Errors or unclear communication?: pending
- More steps than necessary?: pending
- Fits project vision?: expected yes; this item should remain task/card-owned with no review judgment in Rust.
- Backlog-worthy improvements: pending
- No action: pending

## Backlog Emissions

- Added: pending
- Updated: pending
- Proposed: pending

## Closeout

- Final git status: pending
- Remote sync: pending
- Remaining parked tasks: pending
- Remaining DLQ: pending
- Next best pickup: pending
