# Build backlog-chewer cron workflows that start dry-run and graduate to PR-only

Priority: P2 · Status: pending · Estimate: XL

## Goal

Let Bitterblossom cron agents consume whitelisted, well-specced backlogs by first producing execution plans, then later opening reviewed PRs — without auto-selecting vague work or merging by default.

## Oracle

- [ ] A `backlog-chewer-dry-run` task scans only whitelisted repos and selects only tickets with clear Goal, executable Oracle, bounded scope, and allowed credentials/side effects.
- [ ] Under-specified tickets produce a shaping/context-packet artifact instead of implementation.
- [ ] Dry-run mode writes a plan artifact naming selected ticket, assumptions, verifier, budget, and stop conditions; it creates no branch.
- [ ] PR-only mode, after dry-run proves useful, may run the deliver/TDD/review workflow and open a PR but cannot merge.
- [ ] The workflow enforces max one active BB-authored PR per repo/task family and a daily run/cost cap.
- [ ] Fresh-context review and deterministic CI/gates are required before merge eligibility is even reported.
- [ ] `./scripts/verify.sh` passes.

## Verification System

- Claim: BB can chew through ready backlog work without turning vague product direction into autonomous code churn.
- Falsifier: the agent implements an under-specified ticket, chooses outside the whitelist, opens multiple competing PRs, self-grades done, or merges without explicit policy.
- Driver: fixture repo/backlog with ready, vague, blocked, and dangerous tickets; run dry-run and PR-only modes against the fixture.
- Grader: ready ticket selected; vague ticket shaped; blocked/dangerous tickets skipped with reasons; PR-only produces branch/PR and review artifacts but no merge.
- Evidence packet: selection report, plan artifact, PR URL for PR-only smoke, review/gate receipts.
- Cadence: run before expanding repository whitelist.

## Children

1. Define ticket-readiness classifier using deterministic fields plus model-readable context; avoid brittle keyword-only scoring.
2. Add dry-run task/card and fixture backlog.
3. Add PR-only task/card that reuses existing build/review/gate machinery.
4. Add repo whitelist and active-PR pressure checks.
5. Decide later whether any repo earns guarded auto-merge.

## Dry-Run → PR-Only Graduation Metrics

Dry-run is a product requirement, not a delay tactic. PR-only mode is eligible only after dry-run evidence shows the selector and planner are reliable:

- at least 20 dry-run selections across fixture + real whitelisted backlogs;
- 90%+ of selected tickets are judged genuinely ready by a human or fresh reviewer;
- 0 dangerous/blocked tickets are selected for implementation;
- vague tickets produce useful shaping/context packets instead of code attempts;
- every dry-run plan names verifier, acceptance criteria, budget, stop conditions, branch name, and expected changed paths;
- max-one-active-BB-authored-PR checks are implemented before PR-only mode.

Promotion trigger: PR-only can be enabled per repo only when the dry-run scorecard is green for that repo family. Auto-merge remains out of scope until a separate guarded-landing ticket proves repo-specific gates and rollback drills.

## Notes

Why: the operator wants to become primarily a backlog groomer while agents consume shaped work. The safety invariant is that product judgment stays in grooming; BB consumes ready tickets and reports when tickets are not ready.
