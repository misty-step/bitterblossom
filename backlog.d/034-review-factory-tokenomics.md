# Drive review-factory cost to the $1-2/review median

Priority: P2
Status: pending
Estimate: M

## Goal
The review workload (028, shipped 2026-06-10) hits Cloudflare's $1-2
median cost per review without losing finding quality.

## Oracle
- [ ] Median cost over 10+ real reviews is <= $2 (ledger evidence via
      `bb runs list --task review --json`)
- [ ] A trivial (<10 line) diff reviews for <= $0.50
- [ ] Finding quality holds: a seeded-flaw PR still gets all plantings
      flagged at the cheaper configuration

## Notes
Measured baseline 2026-06-10: $2.46 small diff, $3.09 medium (claude
coordinator + subagent fan-out, claude-fable-5 for every lane). The
biggest lever — rebinding the agent to a cheap OpenRouter model on an
open harness — is now ticket 036 (model & auth policy); this ticket is
the measurement gate that proves the median after 036 lands. Remaining
levers here: prompt-cache
reuse across lanes, skipping the bench on trivial diffs (tier-1 already
exists, verify it engages), incremental re-review with prior context
instead of full re-review. Remaining 028 design depth (engine eval via
Daedalus, thinktank fan-out, multi-harness bench, won't-fix dialogue)
rides behind this — cost first, then breadth.

## Update 2026-06-11 (post-036)
The rebind landed: Kimi K2.6 via pi/OpenRouter measured $0.0034 (small,
partial accounting), ~$0.03 (medium, summed), $0.0795 (webhook review of
a 5.6k-addition PR) — 30-90x under the claude baseline and already under
the $1-2 target per run. Remaining oracle work is just the 10+-review
median and the trivial-diff tier check; gather from the ledger as real
reviews accumulate.
