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
- Build runs: `build=f6f2d75b2c3a`, `build-glm=87421671ebd5`,
  `build-kimi=5bccae0c1d4a`
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
- `bb run build`: `f6f2d75b2c3a`, success, `omp` / `z-ai/glm-5.2`,
  cost `$0.51970540`, `165187 / 14125` tokens, duration `393804ms`.
  `REPORT.json` status `dry_run`; recommended keeping OMP/GLM and raising
  the cap to `$4.00`, but correctly avoided editing or pushing in dry-run.
- `bb run build-glm`: `87421671ebd5`, success, `pi` / `z-ai/glm-5.2`,
  cost `$0.20079511`, `52138 / 9651` tokens, duration `317722ms`.
  `REPORT.json` status `dry_run`; cheapest and most actionable report, but
  it claimed local commits during dry-run, which is a contract concern.
- `bb run build-kimi`: `5bccae0c1d4a`, success, `pi` /
  `moonshotai/kimi-k2.7-code`, cost `$0.52021343`, `107868 / 11475` tokens,
  duration `314427ms`. `REPORT.json` status `dry_run`; proposed the same
  `$4.00` cap plus a broader test-backed record.
- `bb run model-eval`: `a6d019b66cda`, success, `openai/gpt-5.5`, cost
  `$0.164629`, `7903 / 4051` tokens, duration `53726ms`.
- Reference context:
  `docs/model-evals/build/2026-06-20-builder-cost-calibration.md`.
- Default decision: keep `build` on OMP / GLM 5.2 and raise
  `max_cost_per_run_usd` from `$2.00` to `$4.00`; keep Pi/GLM as a promising
  comparator, not the default yet.
- Local verification: focused
  `cargo test --test model_eval build_cost_calibration_record_matches_default_cap -- --nocapture`
  passed.
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

- Observation: The cheapest dry-run candidate (`build-glm`) still surfaced a
  dry-run contract concern.
- Evidence: `REPORT.json` for `87421671ebd5` says the branch was committed
  locally while also reporting `status = "dry_run"`.
- Mitigate: Do not promote Pi/GLM to default from this evidence alone. Tighten
  dry-run side-effect expectations later if candidate lanes repeat this.

### Ugly

- Observation: A dry-run comparison can answer cost and planning quality, but
  it cannot prove live branch-authoring behavior.
- Evidence: all three current candidate runs were `dry_run`; the live cost
  pressure still comes from prior authoring run `d19d71f1eeae`.
- Mitigate: Keep the cap raise narrow and record residual risk instead of
  claiming a model-promotion result.

### Friction

- Observation: Sprite org still defaulted to `adminifi`, so the operator had to
  switch before safe dispatch.
- Evidence: `sprite org list` showed `Currently selected org: adminifi`; after
  `sprite use -o misty-step lane-1`, `sprite exec -- whoami` returned `sprite`.
- Mitigate: Keep the mandatory dogfood preflight; consider a future repo-local
  sprite-org preflight only if this repeats enough to justify product surface.

### Bugs

- Observation: None confirmed in this slice.
- Evidence: the earlier mistaken DLQ jq read was operator error; direct
  `dlq list --json` showed the expected old rows.
- Mitigate: no backlog emission.

### Delight

- Observation: The model-eval loop produced a useful adjudication over
  conflicting candidate reports in under a minute.
- Evidence: run `a6d019b66cda` cost `$0.164629`, named scorecards, a winner,
  an accepted conclusion, and residual risk.
- Lean in: Keep the evaluator as the synthesis step when model/cost evidence
  conflicts.

## Reflection

- Does it work?: Yes. `bb` produced three candidate run receipts, preserved
  report artifacts, and ran an evaluator with cost/tokens/duration in the
  ledger.
- Does it produce useful results?: Yes. The result separates an immediate cap
  fix from the future model-promotion question.
- Does it feel good?: Mostly. `bb status --json` made the parked state obvious,
  and model-eval clarified conflicting reports. Long `--json` dispatches still
  require side-channel polling.
- Too complicated / awkward?: The explicit unpark was justified here because
  the parked reason was the ticket subject. The workflow is still many steps,
  but each step left useful receipts.
- Errors or unclear communication?: My first DLQ jq filter was wrong because
  the DLQ JSON does not expose a `status` field. Direct JSON inspection fixed
  the operator error; no product bug filed.
- More steps than necessary?: Not for this decision. The model-eval step paid
  for itself by preventing a premature default switch.
- Fits project vision?: Yes. The change stays in task config and evaluator
  reference docs; no workload judgment moved into Rust.
- Backlog-worthy improvements: no new item yet. If candidate lanes keep
  reporting local commits during `dry_run`, tighten the builder dry-run
  contract with a concrete oracle.
- No action: do not file a sprite-org item from one repeated preflight
  annoyance yet; keep the preflight.

## Backlog Emissions

- Added: none
- Updated: `docs/model-evals/build/README.md`,
  `docs/model-evals/build/2026-06-20-builder-cost-calibration.md`
- Proposed: possible future builder dry-run side-effect guard if repeated

## Closeout

- Final git status: pending
- Remote sync: pending
- Remaining parked tasks: pending final status
- Remaining DLQ: old missing-`GH_TOKEN` rows remain, including IDs `12`, `11`,
  `10`, `9`, and `8`; not replayed in this slice.
- Next best pickup: pending
