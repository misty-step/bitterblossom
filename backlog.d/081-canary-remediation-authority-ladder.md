# Graduate Canary triage through a staged remediation authority ladder

Priority: P2 · Status: pending · Estimate: XL

## Goal

After report-only Canary triage proves useful, add narrowly staged authority for branch creation, PR review, guarded landing, sanity verification, and revert of the agent's own change.

## Oracle

- [ ] Authority levels are explicit in task config/card docs: observe, recommend, branch, guarded land, rollback.
- [ ] Level 1 recommends exact next BB commands but cannot mutate code.
- [ ] Level 2 may create a branch/PR and run tests/review, but cannot merge.
- [ ] Level 3 merge requires deterministic CI/gate, fresh-context review, Canary sanity check, repo allowlist, and an explicit policy gate.
- [ ] Level 4 revert is limited to the agent's own last known change and only after the same incident signature or a declared sanity check still fails.
- [ ] Every level emits run artifacts and operator-visible receipts; failures halt instead of continuing to plow forward.
- [ ] `./scripts/verify.sh` passes.

## Verification System

- Claim: Canary remediation can gain authority incrementally without turning BB into an unbounded production mutation loop.
- Falsifier: any level can skip its verifier, merge without review, revert unrelated code, or continue after no-progress detection.
- Driver: staged fixture incident in a whitelisted repo, with fake/low-risk branches for branch and rollback drills.
- Grader: policy gate enforces allowed actions per level; run artifacts show each decision; no-progress and failed-sanity paths halt.
- Evidence packet: branch/PR URLs, gate JSON, Canary sanity result, revert drill transcript.
- Cadence: each authority level requires its own dogfood evidence before promotion.

## Notes

Why: the operator vision includes investigate → remediate → PR → review → merge → sanity check → revert if unfixed. That is the right long-term loop, but it is unsafe as a first slice. This ticket preserves the ambition while forcing evidence-gated graduation.

Depends on backlog 080 and artifact/observability work from 079/072.
