# Build the next lifecycle reflexes after CI diagnose

Priority: P1 | Status: pending | Estimate: XL

## Goal

Extend the SDLC lifecycle reflex pack beyond the completed CI-diagnose slice
without adding semantic workflow logic to the Rust spine.

## Oracle

- [ ] A `gate.blocked -> fix-prompt-generator` reflex writes a bounded builder
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
2. Add the deploy/prod verifier event payload contract and manual dogfood
   fixture.
3. Decide whether the lifecycle orchestrator is report-only or can create runs
   with a narrow plane token.
4. Add status-surface joins if operators still need to correlate gate state,
   parked tasks, DLQs, and lifecycle recommendations manually.

## Notes

Backlog `061` proved the initial pattern with `check_suite.failed ->
ci-diagnose packet` and a real failed-run model evaluation. This follow-up owns
the remaining SDLC reflex breadth: gate-blocked fix packets, deploy/prod smoke
verification, and deterministic lifecycle orchestration.

Keep the boundary from `061`: the Rust spine routes, leases, budgets, records,
and retries pre-execute. Lifecycle meaning stays in task cards, agent configs,
payload contracts, and reference docs.
