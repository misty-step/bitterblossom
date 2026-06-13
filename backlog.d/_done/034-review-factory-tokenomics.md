# Drive review-factory cost to the $1-2/review median

Priority: P2
Status: done
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

## Closure 2026-06-12

Closed by `review-coordinator@v3`: Kimi K2.6 on OpenRouter with minimal
thinking, bounded source access, shell-safe PR comments, visible final
JSON, and an explicit manual measurement mode that reviews real PR diffs
without posting duplicate GitHub comments. Webhook and normal manual
reviews still post exactly one PR comment.

Evidence packet:
[docs/plans/2026-06-12-034-tokenomics-evidence.md](/docs/plans/2026-06-12-034-tokenomics-evidence.md).

Evidence:

- Config load: `./target/debug/bb --config plane check` showed
  `review-coordinator@v3: pi moonshotai/kimi-k2.6:minimal`.
- Repo gate: `./scripts/verify.sh` completed with `src LOC: 4999` and
  `verify: all gates green`.
- Median: 10 successful `tokenomics:*` review runs under
  `review-coordinator@v3` in the local ledger. Command:
  `./target/debug/bb --config plane runs list --task review --json | jq
  '[.[] | select(.state=="success" and .agent_version==3 and
  (.idempotency_key|startswith("tokenomics:"))) | .cost_usd] | sort |
  {count:length, costs:., median: ((.[4] + .[5]) / 2), max: .[-1],
  min: .[0]}'`.
- Result: `count=10`, costs
  `[0.00512825, 0.00544146, 0.00570033, 0.00734302, 0.00796505,
  0.0103701, 0.01101447, 0.05119053, 0.05575199, 0.06847158]`,
  median `$0.009167575`, max `$0.06847158`.
- Trivial diff: PR #837, 2 changed lines, run `33431118212e`, cost
  `$0.00512825`, duration `24345ms`, `comment_posted:false`.
- Seeded quality: PR #843, run `9cdae182de5f`, cost `$0.06847158`,
  duration `278227ms`; result flagged both plantings:
  `tools/export-metrics.py:36` SQL interpolation and
  `tools/prune-runs.sh:7` unquoted `rm -rf $dir`. PR #843 stayed at 6
  comments before and after measurement.
- No-comment invariant: all 10 v3 measurement artifacts contained
  `"comment_posted": false`.
- Normal-comment invariant: PR #817 normal-mode smoke run
  `01b13f55d7dd` succeeded, cost `$0.01167536`, produced visible JSON,
  used `--body-file REVIEW.md`, and increased PR #817 comments from 2 to
  3.

Hardening found during measurement:

- A v2 run on PR #829 (`7823623d1726`) timed out after `1800s` because
  the reviewer cloned and fetched history. The card now forbids
  clone/fetch/checkout for trivial and standard reviews, allows only
  targeted file reads for large reviews, and treats measurement latency as
  product evidence.
- A normal-mode smoke posted a comment but failed parsing when the model
  placed final JSON only in hidden reasoning. The card now requires a
  visible final JSON object.
- A normal-mode smoke using direct `--body "<review>"` lost markdown code
  spans through shell interpolation. The card now requires `REVIEW.md` plus
  `gh pr comment --body-file REVIEW.md`.
