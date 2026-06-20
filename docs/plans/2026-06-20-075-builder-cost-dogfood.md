# Dogfood 075: Builder Cost Calibration

## Context

- Goal: Calibrate the `build` default after OMP / GLM 5.2 dogfood runs parked
  the task over its per-run cap.
- Backlog item: `backlog.d/075-calibrate-omp-glm-builder-cost.md`
- `bb` binary: `./target/debug/bb`
- Plane: `plane`
- Sprite org: `misty-step`
- Sprite: `lane-1` for `build`; `bb-polisher-3` for `build-glm`;
  `bb-polisher-2` for `build-kimi`.
- Build runs: pending
- PR: pending
- Commit/submission: pending

## Preflight

- `git status`: clean `master...origin/master` before branch creation.
- Branch: `bb/deliver/075-builder-cost-calibration`.
- `flyctl orgs list`: `Misty Step` / `misty-step`.
- `sprite org list` before/after: initially selected `adminifi`; switched to
  `misty-step` with `sprite use -o misty-step lane-1`.
- `sprite exec -- whoami`: `sprite`.
- `bb check`: 28 plane tasks loaded.
- `bb status --json` summary: 28 tasks, cost today `$0.2314450674`, one parked
  task, build parked for `run cost $3.2074 > max_cost_per_run_usd $2.00`.
- `bb task list --json` summary: `build` uses `omp` / `z-ai/glm-5.2`;
  `build-glm` uses `pi` / `z-ai/glm-5.2`; `build-kimi` uses `pi` /
  `moonshotai/kimi-k2.7-code`.
- `bb runs list --json` summary: latest runs are the final 074 gate members
  (`verify=2a36caa531a1`, `correctness=ebf5ba4071aa`,
  `security=cd54e42d417d`, `simplification=583890bfadaa`,
  `product=ecc3e8a19008`), all successful.
- `bb dlq list --json` summary: open rows remain from the older missing
  `GH_TOKEN` storm; recent IDs include `12`, `11`, `10`, `9`, and `8`.
- Live model catalog: `scripts/check-model-catalog.sh --live --json` found
  configured IDs present with no metadata gaps. Current catalog prices:
  `z-ai/glm-5.2` prompt `$0.0000012`, completion `$0.0000041`, context
  `1048576`; `moonshotai/kimi-k2.7-code` prompt `$0.000000612`, completion
  `$0.000003069`, context `262144`.

Build task unpark rationale: `build` is parked for the exact condition this
ticket must measure. Unparking is safe for one explicit calibration dry-run
because today's global spend is far below the `$25.00` daily cap, `build` has
zero runs today, and the expected outcome may re-park the task as evidence.

## Work

- Candidate payload:
  `{"repo":"misty-step/bitterblossom","backlog":"backlog.d/075-calibrate-omp-glm-builder-cost.md","branch_slug":"075-builder-cost-calibration","dry_run":true}`
- `bb run build`: pending
- `bb run build-glm`: pending
- `bb run build-kimi`: pending
- `bb run model-eval`: pending
- Reference context: pending
- Default decision: pending
- Local verification: pending
- `bb submit open`: pending
- `bb run` members: pending
- `bb gate`: pending

## UX Notes

### Good

- Observation: `bb status --json` made the parked build task and exact cap
  reason obvious.
- Evidence: `build.parked = "run cost $3.2074 > max_cost_per_run_usd $2.00"`.
- Lean in: Keep status as the pre-dispatch truth surface for expensive lanes.

### Bad

- Observation: pending
- Evidence: pending
- Mitigate: pending

### Ugly

- Observation: pending
- Evidence: pending
- Mitigate: pending

### Friction

- Observation: Sprite org still defaulted to `adminifi`, so the operator had to
  switch before safe dispatch.
- Evidence: `sprite org list` showed `Currently selected org: adminifi`; after
  `sprite use -o misty-step lane-1`, `sprite exec -- whoami` returned `sprite`.
- Mitigate: Keep the mandatory dogfood preflight; consider a future repo-local
  sprite-org preflight only if this repeats enough to justify product surface.

### Bugs

- Observation: pending
- Evidence: pending
- Mitigate: pending

### Delight

- Observation: pending
- Evidence: pending
- Lean in: pending

## Reflection

- Does it work?: pending
- Does it produce useful results?: pending
- Does it feel good?: pending
- Too complicated / awkward?: pending
- Errors or unclear communication?: pending
- More steps than necessary?: pending
- Fits project vision?: pending
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
