# Enforce task artifact contracts before marking success

Priority: P1 | Status: ready | Estimate: S

## Goal

Stop `bb run` from marking a workload successful when the task's own completion
contract requires an artifact that was not produced.

## Oracle

- [ ] Task config can declare required artifacts, starting with `REPORT.json`
      for report-producing agents such as `build`, `gardener`, `ci-diagnose`,
      model-catalog watch, and model-eval.
- [ ] A run whose harness exits zero but omits a required artifact is recorded
      as failed, with the artifact directory still preserved for inspection.
- [ ] The run result and `bb runs show` name the missing artifact directly.
- [ ] Dry-run/report-only payloads for builder-style tasks are enforceable by
      the plane or task contract, not only by prose in the lane card.
- [ ] Tests cover a zero-exit harness that prints a parseable final message but
      omits `REPORT.json`.
- [ ] `./scripts/verify.sh` passes.

## Notes

Live source: 2026-06-18 OMP/GLM build dry-run dogfood.

```bash
GH_TOKEN=$(gh auth token) ./target/debug/bb --config plane run build \
  --payload '{"backlog":"backlog.d/073-dispatch-readiness-for-subscription-builders.md","branch_slug":"omp-glm-smoke","dry_run":true}' \
  --json
```

Run `b33bbe05b5d9` used `bb-builder-rust@v2` through `omp` /
`z-ai/glm-5.2` and reached ledger state `success` with cost `$2.46364376`,
`289598` input tokens, `30146` output tokens, and artifact dir
`plane/.bb/runs/b33bbe05b5d9/attempt-1`.

The artifact dir contained `stdout.txt`, `stderr.txt`, `result.md`,
`LANE_CARD.md`, `EVENT.json`, and `RUN.json`, but no `REPORT.json`. The lane
card requested a `REPORT.json`, and `dry_run = true` said to inspect and write
a plan/report only, but the agent edited the target checkout for over 20
minutes before emitting a parseable final message. The transport worked; the
completion contract was too weak.
