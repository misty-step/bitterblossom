---
name: canary-responder
description: |
  Handle Canary incident-response workloads on Bitterblossom: re-read Canary
  API truth, resolve the service catalog, diagnose root cause, prepare a
  report or bounded fix plan, and preserve no-merge/no-deploy defaults.
  Use when configuring or running the Tansy canary responder template.
---

# Canary Responder

Run Canary incident response as a Bitterblossom workload, not as Rust spine
logic and not as a revived conductor persona.

## Contract

- Treat webhooks as wake-up hints. Re-read Canary incidents, reports, and
  timeline APIs before choosing a repo or claiming progress.
- Resolve service to repo through `canary-services.toml`. Do not guess from
  service names.
- Diagnose before repair. Separate evidence, hypotheses, root cause, and next
  actions.
- Default to report-only. Merge and deploy authority require explicit catalog
  opt-in plus a payload asking for live action.
- Write a durable `REPORT.json` with incident id, service, repo, evidence,
  root cause status, actions, verification, and residual risk.

## Red Lines

- No production-data re-ingestion into Daedalus or benchmark fixtures.
- No repo mutation when `dry_run` is true.
- No shell interpolation of catalog commands; preserve argv boundaries.
- No success claim before Canary recovery evidence.

## Starter

Use `examples/canary-responder-plane/` as the checked-in template.
