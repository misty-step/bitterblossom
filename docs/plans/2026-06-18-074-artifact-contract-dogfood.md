# Dogfood 074: Task Artifact Contract Enforcement

## Context

- Goal: Use `bb` itself to deliver backlog 074, then critique the build,
  submission, gate, ledger, and operator UX.
- Backlog item: `backlog.d/074-task-artifact-contract-enforcement.md`
- `bb` binary: `./target/debug/bb`
- Plane: `plane`
- Sprite org: `misty-step`
- Sprite: `lane-1`
- Build run: `d19d71f1eeae`
- PR: <https://github.com/misty-step/bitterblossom/pull/862>
- Builder commit: `e87446d86f67d0e9882e48ba13f7d7d0b47c8e22`
- Gate submission: `439333a1ab7c` at rev
  `99e73e6f24909f73bd2949d70924e219e6d4590e`

## Preflight

- `git status`: `## master...origin/master`
- `flyctl orgs list`: `misty-step`
- `sprite org list` before/after: started on `adminifi`; switched with
  `sprite use -o misty-step lane-1`; verified `sprite exec -- whoami` returned
  `sprite`.
- `bb check`: 28 plane tasks loaded successfully.
- `bb status --json` summary: 28 tasks, cost today `$2.46364376`, 1 parked
  task, 10 open DLQ rows.
- `bb task list --json` summary: `build` is `bb-builder-rust@v2`,
  harness `omp`, model `z-ai/glm-5.2`, parked because prior run cost
  `$2.4636 > max_cost_per_run_usd $2.00`.
- `bb runs list --json` summary: latest build run `b33bbe05b5d9` succeeded
  without `REPORT.json`; prior build run `380ca26ed25b` failed on Codex
  subscription auth.
- `bb dlq list --json` summary: open DLQ rows are older submission storm and
  verifier failures, mostly missing `GH_TOKEN`; not touched in this slice.

Build task unpark rationale: the parked reason is the known cost breach from
the OMP/GLM dry-run that produced backlog 074. Unparking is safe only for this
explicit user-requested follow-up run because the operator is accepting one
more spend to address the artifact-contract failure. If the next run breaches
again, leave the task parked and record the cost behavior rather than hiding it.

## Work

- `bb run build`: run `d19d71f1eeae`, state `success`, duration 673.322s,
  cost `$3.207397`, tokens `288427` in / `18850` out, 57 turns.
- Build `REPORT.json`: present at
  `plane/.bb/runs/d19d71f1eeae/attempt-1/REPORT.json`; status `ready`,
  branch `bb/build/074-artifact-contract`, commit
  `e87446d86f67d0e9882e48ba13f7d7d0b47c8e22`.
- Branch checkout: `git fetch origin && git checkout bb/build/074-artifact-contract`
  created a local tracking branch from `origin/bb/build/074-artifact-contract`.
- PR: draft PR #862, "Enforce task artifact contracts before success".
- Local verification: `./scripts/verify.sh` passed twice locally; second pass
  after adding spine docs reported all gates green and LOC `5149 <= 6000`.
- `bb submit open`: submission `439333a1ab7c`, round 1, opened against
  PR #862 at rev `99e73e6f24909f73bd2949d70924e219e6d4590e`.
- `bb run` members:
  - `verify`: run `9ebb112c92df`, verdict pass, `./scripts/verify.sh`
    green in sprite, LOC `5149`.
  - `correctness`: run `faecaaf7346f`, verdict pass, cost `$0.058385787`.
  - `security`: run `33580b43fb4d`, verdict pass, cost `$0.016741352`.
  - `simplification`: run `be3696122eb4`, verdict pass, cost
    `$0.0065582444`.
  - `product`: run `d4b7dc371e08`, verdict pass, cost `$0.07034905`.
- `bb gate`: `decision = clear`; no blocking, advisory, or rejected findings.

## UX Notes

### Good

- Observation: `bb status --json` makes the parked build task and exact cost
  reason obvious before dispatch.
- Evidence: `build.parked = "run cost $2.4636 > max_cost_per_run_usd $2.00"`.
- Lean in: Keep status as the operator truth surface before expensive runs.
- Observation: The OMP/GLM builder produced a proper `REPORT.json` on this run,
  and the run bundle captured cost/tokens/turns without extra provider calls.
- Evidence: attempt artifact dir includes `REPORT.json`; run `d19d71f1eeae`
  records `$3.207397`, `288427` input tokens, `18850` output tokens, 57 turns.
- Lean in: Keep artifact dir + ledger stats as the primary receipt.

### Bad

- Observation: The product can strand the primary builder after a successful
  but over-budget dogfood run; the safe recovery is a human judgment call.
- Evidence: build task parked even though latest build state is `success`.
- Mitigate: This run will record whether 074 needs to include better
  post-breach recovery guidance.
- Observation: The OMP/GLM builder is effective but expensive for a small
  Rust slice.
- Evidence: backlog 074 implementation run cost `$3.207397`, breaching the
  configured `$2.00` advisory cap; `bb status --json` re-parked `build` with
  reason `run cost $3.2074 > max_cost_per_run_usd $2.00`.
- Mitigate: Record as dogfood evidence; later model-eval should compare OMP
  GLM with Pi GLM and Kimi on the same build slice before treating OMP as a
  universal default.

### Ugly

- Observation: The run-list CLI invited a natural but unsupported bounded-list
  flag during live triage.
- Evidence: `bb runs list --limit 10 --json` failed with
  `unexpected argument '--limit'`; the workaround was `bb runs list --json |
  jq 'sort_by(.updated_at) | reverse | .[0:12]'`.
- Mitigate: Do not file a new item from this one-off. If it repeats, fold it
  into the existing operator/API shape work rather than adding another CLI
  papercut ticket.

### Friction

- Observation: Sprite org defaulted to `adminifi`, requiring explicit switch
  before dogfood could safely run.
- Evidence: `sprite org list` before `sprite use` showed selected org
  `adminifi`; after switch it showed `misty-step`.
- Mitigate: Keep the hard preflight. Consider future repo-local sprite
  selection checks if this repeats.
- Observation: `bb run build --json` is completely quiet until the run ends,
  so active progress required a second status/sprite inspection loop.
- Evidence: run `d19d71f1eeae` only printed final JSON after completion; live
  progress came from `bb runs show`, `bb status --json`, and `sprite exec ps`.
- Mitigate: Human mode already has heartbeats; keep recommending non-JSON for
  supervised long runs, or add an agent-readable follow mode later.

### Bugs

- Observation: Prior follow-up run `b33bbe05b5d9` proved success could omit
  required `REPORT.json`; this run's branch claims to fix that at the plane
  level.
- Evidence: builder report says zero-exit parseable output without
  `REPORT.json` is now recorded as failure and names the missing artifact.
- Mitigate: Verify locally before opening PR; run submission gate after PR.

### Delight

- Observation: `sprite use -o misty-step lane-1` plus `sprite exec -- whoami`
  gives a simple, direct substrate sanity check.
- Evidence: remote command returned `sprite`.
- Lean in: Preserve this as a mandatory dogfood preflight.

## Reflection

- Does it work?: Yes for build dispatch: `bb` produced a branch, commit,
  `REPORT.json`, ledger rows, artifacts, costs, and a local green gate.
- Does it produce useful results?: Yes. The diff is narrow: task config
  declares required artifacts, dispatch enforces them after release, and local
  tests cover missing/present artifact cases.
- Does it feel good?: Mixed. State and receipts are good; the primary builder
  parking itself after every successful OMP/GLM run is noisy and slows the loop.
- Too complicated / awkward?: The mandatory status and sprite-org preflight is
  justified, but long `--json` runs need side-channel monitoring.
- Errors or unclear communication?: The cost park is clear, but deciding when
  it is safe to unpark is still operator judgment.
- More steps than necessary?: PR/submission/gate remain intentionally explicit;
  the only unnecessary-feeling step was monitoring quiet `--json` progress.
- Fits project vision?: Yes. The fix keeps completion truth in task config and
  dispatch mechanics, not workload-specific Rust judgment.
- Backlog-worthy improvements: OMP/GLM build cost calibration after more than
  one over-budget success.
- No action: Sprite org switch remains a preflight practice for now; not enough
  new evidence for a separate backlog item.
- No action: Quiet `bb run --json` was frustrating for supervision, but it is
  already an intentional contract after prior heartbeat work: human mode emits
  progress, JSON mode preserves a clean final bundle. Use ledger polling when a
  human supervises JSON-mode cohorts.

## Backlog Emissions

- Added: `backlog.d/075-calibrate-omp-glm-builder-cost.md`
- Updated: none
- Proposed: none

## Closeout

- Final git status: this notes file and the new backlog item were intentionally
  left for the operator closeout commit after the first submission gate cleared.
  The final response records the post-commit status and remote-sync result.
- Remote sync: pending final closeout.
- Remaining parked tasks: `build` re-parked after run `d19d71f1eeae` for
  `run cost $3.2074 > max_cost_per_run_usd $2.00`.
- Remaining DLQ: 10 open rows at preflight; not replayed or resolved in this
  slice.
- Next best pickup: backlog 075, to decide whether the OMP/GLM build default
  needs a higher cap, a different default, or a clearer split between cheap
  dry-run and expensive live builders.
