# Build the next lifecycle reflexes after CI diagnose

Priority: P1 | Status: pending | Estimate: XL

## Goal

Extend the SDLC lifecycle reflex pack beyond the completed CI-diagnose slice
without adding semantic workflow logic to the Rust spine.

## Oracle

- [x] A `gate.blocked -> fix-prompt-generator` reflex writes a bounded builder
      packet from gate findings without editing code, resolving runs, parking
      tasks, or merging.
- [ ] A deploy/prod verifier reflex consumes a deploy-smoke or production
      incident payload and writes concrete browser/API evidence plus a suggested
      next run.
- [ ] A lifecycle orchestrator task reads event payload plus plane state and
      emits deterministic follow-up `bb run` commands or a run plan artifact.
- [ ] Each reflex has manual dogfood payloads, task/agent/card files, report
      artifact shape tests, and `bb` receipts with run ids and costs.
- [ ] Any model-selection decision uses at least three candidate configs plus
      `model-eval` reference context before promotion.
- [ ] `./scripts/verify.sh` passes.

## Children

1. ~~Shape the `gate.blocked` fix-prompt report contract and red lines.~~ →
   **graduated to backlog 070** (ready, P1) as the cheapest, highest-leverage
   first reflex; budget-free (the `gate.blocked` decision already exists).
2. [x] Add the deploy/prod verifier event payload contract and manual dogfood
   fixture.
3. [x] Decide whether the lifecycle orchestrator is report-only or can create runs
   with a narrow plane token.
4. [x] Add status-surface joins if operators still need to correlate gate state,
   parked tasks, DLQs, and lifecycle recommendations manually.

## Notes

Backlog `061` proved the initial pattern with `check_suite.failed ->
ci-diagnose packet` and a real failed-run model evaluation. This follow-up owns
the remaining SDLC reflex breadth: gate-blocked fix packets, deploy/prod smoke
verification, and deterministic lifecycle orchestration.

Keep the boundary from `061`: the Rust spine routes, leases, budgets, records,
and retries pre-execute. Lifecycle meaning stays in task cards, agent configs,
payload contracts, and reference docs.

## Delivery Notes

### 2026-07-02 deploy/prod verifier fixture

- Added the `deploy-prod-verify` public-plane task and `prod-verifier` agent as
  a report-only deploy-smoke / production-incident verifier.
- Added the pinned `bb.deploy_prod_verifier_event.v1` schema and valid manual
  dogfood payload fixture.
- Added lifecycle tests for manual/webhook shape, dedupe/filtering, red lines,
  and report fields, plus contract tests for the event fixture.
- Verification:
  `cargo test --test deploy_verifier_contract --test lifecycle_reflex --test task_card_contract`.

### 2026-07-02 lifecycle orchestrator authority

- Decided the lifecycle orchestrator remains report-only: it may write exact
  follow-up `bb run ... --payload-file ...` commands and run-plan artifacts, but
  does not receive a plane mutation token.
- Documented the allowed report shape, red lines, and revisit criteria in
  `docs/lifecycle-orchestrator-authority.md`.

### 2026-07-02 lifecycle status read path

- Decided no new runtime status join is needed until a real lifecycle
  orchestrator run proves operators need to stitch together more than source
  run id, follow-up run id, and submission key.
- Documented the current read path in `docs/lifecycle-status-read-path.md`:
  `bb status --json`, `bb runs show <run-id> --json`,
  `bb artifacts read <run-id> REPORT.json --json`, and `bb gate --json` when a
  recommendation references a submission.

### 2026-07-02 fix-prompt reflex contract slice

- Added a public-plane `fix-prompt` report-only task, `fix-prompt-generator`
  API-auth agent, and card for signed `gate.blocked` webhook replay or manual
  dispatch.
- Added a valid `gate.blocked` event fixture and `bb.fix_prompt_report.v1`
  report fixture; the lifecycle tests assert that every blocking fingerprint,
  file, line, claim, and evidence string survives into the report and bounded
  builder packet.
- Added webhook filter/dedupe tests for `/hooks/fix-prompt` and task red-line
  tests proving no action fan-out and no mutation authority in config/card.
- Verification:
  `cargo test --locked --test lifecycle_reflex --test task_card_contract -- --nocapture`
  and `cargo run --quiet -- --config tests/fixtures/public-plane check`.
- Deferred by overnight guardrail: no live Sprite run receipt/cost was produced
  because overnight mode forbids Sprite dispatches.

### 2026-07-02 lifecycle orchestrator contract slice

- Built the report-only `lifecycle-orchestrator` task that the 2026-07-02
  authority decision only specified: added the public-plane
  `lifecycle-orchestrator` task + card + `lifecycle-orchestrator` API-auth agent,
  and the pinned `bb.lifecycle_orchestrator_report.v1.valid.json` fixture.
- The card enforces the authority contract from
  `docs/lifecycle-orchestrator-authority.md`: read `RUN.json`/`EVENT.json` + plane
  read surfaces, emit `recommended_runs` (each an exact `bb run ... --payload-file
  ... --idempotency-key ... --json` step), `stop_conditions`, `plane_snapshot`,
  and `residual_risk`; report-only red lines forbid mutations and any
  merge/deploy/unpark/resolve/dlq/notify command inside the generated plan.
- Manual-only for this slice: the webhook/cron auto-trigger cadence is a
  deliberate later decision, not baked in.
- Added lifecycle tests asserting the task config/card red lines and that the
  report fixture is a report-only run plan whose every step is a `bb run` with an
  idempotency key and no forbidden mutation.
- Verification:
  `cargo test --locked --test lifecycle_reflex --test task_card_contract lifecycle_orchestrator`
  and `cargo run --quiet -- --config tests/fixtures/public-plane check` (lists the
  new task). `./scripts/verify.sh` green.
- Deferred by overnight guardrail: no live Sprite run of the orchestrator was
  produced, so the oracle item stays open pending a supervised live plan run.
