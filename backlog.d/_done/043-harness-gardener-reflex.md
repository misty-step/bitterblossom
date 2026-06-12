# Build the harness-gardener reflex: mine the ledger, improve the system

Priority: P3 · Status: done · Estimate: M

## Goal

Quality-per-token compounds on its own: a standing reflex mines the
verdict/run corpus for recurring patterns and emits concrete harness
improvements (card amendments, new gates, lint rules, reviewer
rebindings) as backlog tickets — "improving the agentic engineering
system" becomes a workload, not a thing the operator remembers to do.

## Oracle

- [x] A cron-triggered task reads the ledger (runs, verdicts, costs,
      dispositions) over a window and produces a structured report:
      recurring finding categories, reviewers whose blocking findings
      are repeatedly overruled by arbiters, demotion/rejection abuse
      signals, cost outliers per task
- [x] At least one recommendation per non-dry-run with enough evidence is
      concrete and mechanical ("add X to CLAUDE.md / this lint / this
      trigger filter"), filed as a backlog.d ticket with evidence
      pointers into the ledger — findings without a falsifiable
      improvement are dropped, not filed
- [x] The gardener cannot modify the harness directly: it files tickets
      only (red line in the card) — the operator or a dispatch lane
      ratifies
- [x] Run cost ≤ $0.25 per weekly sweep (cheap-model binding)

## Notes

**Why:** the meta-loop from the agent-first SDLC brainstorm
(2026-06-11): the verdict storm (039) generates a corpus — every
finding, disposition, oscillation, and cost. Unmined, it's storage;
mined, it's the feedback loop the operator's goal demanded ("define and
run verification and feedback loops"). Also the audit mechanism named
in 039's risks (demotion/rejection abuse) and 041's trust notes.
**Blocked by 039** — without verdicts there is no corpus; runs/costs
alone are too thin to justify the loop.

The original note expected pure config only. Delivery exposed one generic
spine gap: remote agents could read runs/tasks/DLQ but not the verdict
corpus. The shipped spine change is a read-only `GET /api/submissions`
surface returning submissions with verdicts and rejection reasons; no
gardener-specific workload logic moved into Rust.

## Evidence (2026-06-12)

- Config: `plane/agents/gardener.toml` binds `pi` to
  `deepseek/deepseek-v4-flash`; `plane/tasks/gardener/task.toml` adds a
  manual trigger plus weekly cron (`0 15 * * 1`), sprites substrate, and
  `max_cost_per_run_usd = 0.25`.
- Card: `plane/tasks/gardener/card.md` reads `/api/runs`, `/api/tasks`,
  `/api/dlq`, and `/api/submissions?limit=200`; filters a rolling UTC
  `window_days`; files exactly one draft backlog PR only for concrete
  non-dry-run recommendations; dry-run writes `REPORT.json` and never
  clones/pushes/opens a PR.
- Test: `cargo test --test submission
  list_submissions_includes_verdict_rows_for_gardener_api` passed,
  proving the gardener API bundle includes verdict rows and named
  rejection reasons.
- API smoke: temp `bb serve` on `127.0.0.1:7877` with
  `BB_API_TOKEN=qa-token`; unauthenticated `/api/submissions` returned
  `401`; authorized `/api/submissions?limit=1` returned seeded
  submission `1df416a3c43b` with `verdicts=[]` and `rejections=[]`.
- Live dry-run: temp local-substrate gardener plane using the checked-in
  card and real `pi` harness ran `ed564d5abfde` successfully against a
  temp API at `127.0.0.1:7878`; cost `0.0053456032`, duration
  `104334ms`, model `deepseek/deepseek-v4-flash`. It inspected
  submission `aa71c9f281e0`, wrote `REPORT.json`, printed `DRY_RUN`,
  and filed no ticket because the data was intentionally insufficient.
- Gate: `./scripts/verify.sh` green; final `src LOC: 4999`.
- Fresh review: Grok initial review found window/runtime-shape gaps; the
  card gained `window_days`, `api_url`, and `dry_run`. Final Grok
  re-review reported `CLEAN`; residual risk is the non-dry-run
  sprite/GitHub PR filing path, which was not executed locally because
  this shell lacks `BB_API_TOKEN`, `GH_TOKEN`, and `SPRITE_TOKEN`.
