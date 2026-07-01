# Epic: skill-to-agent flows for docs and CI audit loops

Priority: P2 | Status: ready | Estimate: XL

## Goal

Make Harness Kit skills executable as focused BB workloads, starting with the
Weave doc-sync and CI-auditor flows. Skills remain agent guidance; BB supplies
triggers, budgets, scoped keys, artifacts, and PR-only execution.

## Oracle

- [ ] A doc-sync BB task runs the `document` skill on PR merge or daily for
      managed repos, using a cheap model and per-agent budget/key.
- [ ] A CI-auditor BB task audits repo gates/tests/lints and proposes PRs that
      improve enforcement, speed, or cost without lowering gates.
- [ ] Both tasks produce structured artifacts naming source repo, trigger,
      skill version, model, cost, changed files, evidence, and residual risk.
- [ ] Both tasks can run report-only and PR-only modes; merge remains human or
      separately scorecard-gated.
- [ ] At least two repos receive useful doc-sync PRs in one week, and at least
      three gate improvements merge in one month before authority expansion.
- [ ] Model choice is later justified by Crucible measurement; before that, BYOK
      cheap OpenRouter models are explicitly provisional.
- [ ] `./scripts/verify.sh` passes.

## Children

- [ ] Doc-sync task/card/agent, aligned with Weave backlog 005.
- [ ] CI-auditor task/card/agent, aligned with Weave backlog 006.
- [ ] Skill version/provenance capture in artifacts.
- [ ] Report-only to PR-only authority scorecards.
- [ ] Managed-repo allowlist and per-agent budget/key setup.

## Notes

Weave backlog 005 and 006 define the cross-repo outcomes. BB owns the execution
plane half: scheduling, scoped authority, cost, artifacts, and reviewable PRs.
