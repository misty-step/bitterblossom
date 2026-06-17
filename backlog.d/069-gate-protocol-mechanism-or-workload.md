# Decide whether the submission/gate protocol is spine mechanism or workload

Priority: P2 | Status: ready | Estimate: M

## Goal

Resolve whether `src/submit.rs` (submission / gate / verdict / round / arbiter,
~510 lines, ~10% of the spine) is plane MECHANISM or the review WORKLOAD's
judgment living in the core — and if it's workload, extract it so the spine
shrinks.

## Why

The spine LOC cap is a proxy for one invariant: `src/` is mechanism, not
workload judgment (CLAUDE.md). When 067 hit the cap it was re-baselined
(5000 -> 5300) because the spine was verifiably lean and nothing was obviously
workload. But `submit.rs` is the largest single candidate: "adversarial review
storm with rounds, fingerprint dedup, and arbiters" reads like a specific
workload pattern, and #860's review flagged submission-lifecycle logic forking
into `ingress.rs`. If it's workload, extracting it is the real lever for a
smaller spine — far more than line-golfing.

## Oracle

- [ ] A written decision: general plane primitive (many workloads gate) vs
      review-workload judgment.
- [ ] If workload: a plan to move it out (where, how `bb gate`/dispatch refer
      to it) without breaking the live review storm.
- [ ] If mechanism: document WHY it belongs in the spine so the question stops
      recurring whenever the budget is tight.
- [ ] The spine LOC cap reflects the outcome (drops if extracted).

## Notes

Surfaced 2026-06-17 during the 067 budget discussion. Architecture
investigation, not a quick edit — route through `/shape` before building.
