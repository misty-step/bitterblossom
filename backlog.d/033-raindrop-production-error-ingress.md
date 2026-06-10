# Evaluate Raindrop as the production-error ingress for the diagnose watcher

Priority: P2
Status: pending
Estimate: M

## Goal

Decide, with a live trial on one AI-feature product, whether Raindrop 2.0
(raindrop.ai) is the event source that feeds the production-error →
diagnose/fix/postmortem workflow on this plane.

## Why

The Mode B roadmap has a monitor watcher after the review factory. Raindrop
is purpose-built for the half of monitoring that exception trackers miss:
behavior-level signals (custom classifiers for hallucination, tool failure,
refusal, user frustration) over production agent runs, auto-detected
issues, and an MCP surface for auto-fix loops — i.e., the self-healing loop
we'd otherwise hand-build. Sentry-style trackers fire on crashes; agents
fail confidently without crashing.

## Notes

- Trial on one product repo with a real AI feature; instrument with the
  SDK, define 2–3 signals, see what issues surface in a week.
- The watcher shape: Raindrop issue fires → diagnose lane with the trace as
  the lane card's evidence input → fix PR or postmortem ticket. Subject to
  the loop guardrails contract (harness-kit `meta/CONTRACTS.md` §6).
- Compare against the degenerate alternative: structured agent logs +
  scheduled triage loop on the spine (031). Raindrop must beat that on
  signal quality or setup cost to earn a vendor dependency.
- Workshop (their open-source local tool) may be the cheap on-ramp.

## Oracle

- [ ] One product instrumented; at least one real behavior-level issue
      surfaced that exception logging would have missed (or a documented
      finding that none occurred — also an answer).
- [ ] Written keep/drop verdict comparing against the logs+triage-loop
      alternative, with cost.
- [ ] If keep: watcher ticket filed with the Raindrop-issue → diagnose-lane
      contract; if drop: this ticket archived with the verdict.
