# Build the harness-gardener reflex: mine the ledger, improve the system

Priority: P3 · Status: blocked · Estimate: M

## Goal

Quality-per-token compounds on its own: a standing reflex mines the
verdict/run corpus for recurring patterns and emits concrete harness
improvements (card amendments, new gates, lint rules, reviewer
rebindings) as backlog tickets — "improving the agentic engineering
system" becomes a workload, not a thing the operator remembers to do.

## Oracle

- [ ] A cron-triggered task reads the ledger (runs, verdicts, costs,
      dispositions) over a window and produces a structured report:
      recurring finding categories, reviewers whose blocking findings
      are repeatedly overruled by arbiters, demotion/rejection abuse
      signals, cost outliers per task
- [ ] At least one recommendation per run is concrete and mechanical
      ("add X to CLAUDE.md / this lint / this trigger filter"), filed as
      a backlog.d ticket with evidence pointers into the ledger —
      findings without a falsifiable improvement are dropped, not filed
- [ ] The gardener cannot modify the harness directly: it files tickets
      only (red line in the card) — the operator or a dispatch lane
      ratifies
- [ ] Run cost ≤ $0.25 per weekly sweep (cheap-model binding)

## Notes

**Why:** the meta-loop from the agent-first SDLC brainstorm
(2026-06-11): the verdict storm (039) generates a corpus — every
finding, disposition, oscillation, and cost. Unmined, it's storage;
mined, it's the feedback loop the operator's goal demanded ("define and
run verification and feedback loops"). Also the audit mechanism named
in 039's risks (demotion/rejection abuse) and 041's trust notes.
**Blocked by 039** — without verdicts there is no corpus; runs/costs
alone are too thin to justify the loop. Pure config on the spine: one
task.toml + card, no spine changes expected.
