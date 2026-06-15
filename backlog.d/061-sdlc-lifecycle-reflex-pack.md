# Add an SDLC lifecycle reflex pack

Priority: P1 | Status: ready | Estimate: XL

## Goal

Make Bitterblossom a durable SDLC loop plane, not just an ad-hoc dispatch and
post-hoc review runner: predefined lifecycle events should trigger focused
reflex agents that review, verify, diagnose, generate fix packets, and leave
durable receipts.

## Oracle

- [ ] A lifecycle design doc names the event graph, tasks, agents, payloads,
      budgets, and red lines for PR-ready, submission-opened, CI-failed,
      gate-blocked, deploy-smoke-failed, and production-incident events.
- [ ] At least one runnable reflex slice exists beyond the current PR review
      webhook: either `submission.opened -> storm members` or
      `check_suite.failed -> ci-diagnose packet`.
- [ ] The slice is implemented as task/agent/card/config files plus existing
      spine mechanics, with no workload-specific Rust branches.
- [ ] Follow-up run commands or run creation are deterministic and auditable;
      an LLM may recommend the next run, but the plane records the exact
      command/payload/event that caused it.
- [ ] All reflex agents write a durable report artifact with event, repo, rev,
      claim, evidence, suggested next action, cost, artifact paths, and
      residual risk.
- [ ] `bb status --json` or a documented operator read shows lifecycle runs,
      parked reasons, open DLQs, and pending gate members in one reviewable
      path, or the exact status-surface follow-up is filed.
- [ ] `./scripts/verify.sh` passes, and a real `bb submit`/`bb gate` dogfood
      run records the new reflex behavior or the external blocker.

## Shape

Treat SDLC reflexes as a **template pack plus live plane tasks**, not as a Rust
workflow engine.

Initial reflex agents:

- `lifecycle-orchestrator`: reads lifecycle events and plane state; emits a
  deterministic run plan or exact follow-up `bb run` commands.
- `ci-diagnoser`: consumes failed check logs and writes a fix packet.
- `fix-prompt-generator`: converts gate findings into a bounded builder packet.
- `prod-verifier`: runs browser/API smoke against a deployed target and reports
  concrete breakage.

Existing agents remain in scope:

- `review-coordinator` for PR webhook review.
- `verifier` for deterministic gates.
- `correctness`, `security`, `simplification`, and `product` for independent
  verdict storm lanes.
- `gardener` for ledger-mined improvement tickets.

Model posture:

- Use cheap OpenRouter/Pi reflex lanes by default.
- Use DeepSeek V4 Flash for high-volume triage/simplification.
- Use DeepSeek V4 Pro for long-context correctness/security.
- Use Kimi K2.7 Code for coding-aware orchestration where local smoke evidence
  exists.
- Treat GLM 5.2 as page-visible/API-pending until it appears in the API catalog
  and passes a local harness smoke.
- Use Fusion only for architecture/research council questions, not routine
  coding or deterministic gates.

## Children

1. Write the lifecycle event graph and payload contract.
2. Pick the first runnable slice: `check_suite.failed -> ci-diagnose packet`.
3. Add the needed task/agent/card files and dev-plane validation fixtures.
4. Add report artifact shape tests for the selected reflex task.
5. Dogfood on a real Bitterblossom PR and record run IDs, costs, findings,
   parked tasks, and operator UX notes.
6. File follow-ups for status joins or run-creation permissions discovered
   during dogfood.

## Slice 1: CI diagnose reflex

The first runnable slice is `check_suite.failed -> ci-diagnose packet`.
Acceptance is intentionally data-owned: a `ci-diagnoser` API-auth agent, a
`ci-diagnose` task with manual and GitHub webhook triggers, fail-closed
check-suite filters, and a card that writes a durable diagnosis/fix packet to
`REPORT.json`. The plane records the exact trigger payload and run id; the
agent may recommend a builder run, but it does not create that run, push code,
comment, merge, deploy, park tasks, or resolve runs.

## Notes

Why now: the builder-dispatch slice proved manual implementation dispatch can
be a first-class task, but the user goal is an Amjad/Replit-style loop where
the orchestrator prompts parallel agents, verifier feedback generates fix
packets, and specialized security/production agents trigger at lifecycle
boundaries.

Research packet:
[docs/plans/2026-06-15-sdlc-reflex-agent-plane.md](/docs/plans/2026-06-15-sdlc-reflex-agent-plane.md).

Keep these boundaries:

- The Rust spine routes, leases, budgets, records, retries pre-execute, and
  gates. It does not learn SDLC semantics.
- Lifecycle structure should be deterministic and diffable.
- Reflex agents use API auth and hermetic execution; subscription auth remains
  manual dispatch only.
- More agents are not automatically better. Use multi-agent storming for
  independent review domains, breadth research, and verification; keep tightly
  coupled fixes in one builder lane.
