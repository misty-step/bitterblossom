# Epic: Canary triage and remediation on BB

Priority: P1 | Status: ready | Estimate: XL

## Goal

Turn Canary incidents into BB-run evidence and, after report-only proof, bounded
fix PRs. Merge stays human/advisory-gated until a later scorecard proves higher
authority is safe.

## Oracle

- [ ] Canary incident webhook payloads can trigger a `canary-triage` BB task with
      service, repo, environment, fingerprint, severity, and evidence links.
- [ ] Report-only mode writes `REPORT.json` with investigation summary, cited
      evidence, likely files/services, suggested fix path, cost, and uncertainty.
- [ ] Report-only mode has no external side effects beyond the configured
      notification/artifact.
- [ ] PR-only remediation mode is a separate authority level with explicit
      scorecard gates, max one active PR per incident, branch naming, and review.
- [ ] A fixture incident and one real low-severity incident run end-to-end.
- [ ] `./scripts/verify.sh` passes.

## Children

- [ ] Stabilize Canary incident payload and BB fixture.
- [ ] Report-only `canary-triage` task/card/agent budget.
- [ ] Notification and artifact linking back to the incident.
- [ ] Authority-ladder scorecard: report -> PR-only -> guarded rollback.
- [ ] PR-only remediation smoke in a low-risk repo.

## Notes

This epic extends existing Canary triage backlog work without silently promoting
authority. The first useful product is evidence and a suggested next command,
not autonomous mutation.
