# Build Canary incident triage as a report-only Bitterblossom workflow

Priority: P1 · Status: pending · Estimate: XL

## Goal

Make Canary incidents trigger a bounded Bitterblossom triage agent that investigates code and infrastructure context, writes hypotheses and evidence to `REPORT.json`, and recommends next actions without mutating code or production.

## Oracle

- [ ] Canary can emit or replay a stable incident payload containing service, repo mapping or lookup key, environment, error fingerprint, timestamps, severity, and relevant logs/traces/links.
- [ ] Bitterblossom has a `canary-triage` task/agent/card with manual and webhook dogfood paths.
- [ ] The workflow is `report_only`: no code edits, no branches, no PRs, no merges, no deploys, no task parking/unparking, and no run resolution.
- [ ] The triage agent materializes the target repo/infrastructure context and writes `REPORT.json` with incident summary, evidence, hypotheses, likely owner files/services, suggested next BB commands, and residual uncertainty.
- [ ] Status/API/notification surfaces make the incident run and artifact visible without SSH/log spelunking.
- [ ] A replayed fixture incident produces a useful report and no external side effects.
- [ ] `./scripts/verify.sh` passes.

## Verification System

- Claim: Canary incidents can become BB run evidence without giving the agent mutation authority.
- Falsifier: the task edits code, posts publicly, merges, deploys, needs undocumented Canary fields, or cannot link the report back to the incident.
- Driver: fixture Canary incident payload through manual `bb run canary-triage --payload-file ... --json`, then webhook replay once Canary emits the event.
- Grader: report covers the incident with cited evidence; no repo diff exists; no external side effects recorded; artifact readable through backlog 079 surfaces.
- Evidence packet: incident fixture, run id, `REPORT.json`, status JSON, and notification transcript.
- Cadence: run on every Canary payload schema change and before increasing authority.

## Children

1. [x] Define the Canary incident payload and service→repo mapping contract; file Canary-side work if the event is missing fields.
2. [x] Add manual fixture and report-only `canary-triage` BB task.
3. [x] Add webhook trigger with containment filters and budget caps.
4. [x] Add docs/skill recipe for incident triage.
5. Dogfood on a real low-severity Canary event before any mutation authority.

## Report-Only Graduation Metrics

This ticket must ship with an explicit promotion scorecard. It remains report-only until all of these are true:

- at least 5 replayed fixture incidents and 3 real low-severity incidents produce useful `REPORT.json` artifacts with no external side effects;
- every report links incident id, service→repo mapping, evidence sources, suspected files/services, confidence, and residual uncertainty;
- human or fresh-agent review marks at least 80% of reports “actionable” and 0 reports “dangerously wrong”;
- artifact access via backlog 079 is sufficient to inspect reports without SSH/path spelunking;
- alert noise stays bounded: no duplicate storm for the same incident fingerprint within the configured dedupe window;
- budget/cost per incident is visible in `bb status`/runs and within the configured cap.

Promotion trigger: only when the scorecard is green should backlog 081 level 2 (“branch/PR but no merge”) become eligible. Any mutation authority must cite the report-only evidence packet that justified promotion.

## Notes

Why: Canary triage is the highest-value first unsupervised workflow because it is event-native, evidence-heavy, and can start report-only. Remediation and rollback belong in later authority levels, not this ticket.

Swarm evidence 2026-06-29: `/Users/phaedrus/Development/bitterblossom/canary-services.toml` already maps `canary` / `canary-triage` to `misty-step/canary` with `auto_merge = false` and `auto_deploy = false`, enough for an MVP service→repo lookup. Start with manual fixture replay; webhooks only wake the responder and the responder should query Canary for source-of-truth incident state.

Canary-side references to inspect before implementation: `/Users/phaedrus/Development/canary/backlog.d/010-ramp-pattern.md` for the current responder north star and `048-responder-rich-context-safety-gate.md` before broader responders or write-back. Older `011-canary-triage-sprite` references appear stale/archived.

Mode B readiness: repeats on incidents; verifier is report quality plus later human/fresh agent review; environment is target repo + infra context; budgets and blast radius are bounded by report-only authority.

## Delivery Notes

### 2026-07-02 report-only contract hardening

- Tightened the public-plane `canary-triage` card to forbid code edits,
  branches, PRs, merges, deploys, remediation claims, Canary annotations,
  incident acknowledgement/resolution, task parking/unparking, run resolution,
  and user-visible notifications.
- Added the pinned `bb.canary_triage_report.v1.valid.json` fixture using
  schema `bb.canary_incident_response.report.v1`. It links incident id, service,
  environment, severity, fingerprint, replay URLs, service→repo mapping source,
  hypotheses with owner files/services, recommended next commands, and
  `constraints.mutations_performed = []`.
- Added contract tests proving the report is actionable and report-only, and
  lifecycle tests proving the task/card still exposes the expected API-auth,
  Sprite, webhook, budget, and no-mutation contract.
- Verification:
  `cargo test --locked --test canary_contract --test lifecycle_reflex --test ingress --test task_card_contract -- --nocapture`.
- Deferred by overnight guardrail: no live Sprite dispatch, run receipt, cost,
  or real low-severity Canary event dogfood was produced.
