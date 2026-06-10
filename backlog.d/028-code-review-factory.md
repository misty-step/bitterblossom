# Build the event-driven code review factory (absorbed Cerberus mission)

Priority: P1
Status: pending
Estimate: XL

> Groom 2026-06-10: this is workload #1 on the v3 Rust event-plane spine
> (031). The spine carries ingress/ledger/queue/substrate; this ticket
> keeps the review-specific design (coordinator, tiering, tokenomics).

## Goal

A CI/event-triggered code review system on the bitterblossom plane:
coordinator + specialized reviewers that post one structured review per
PR/MR, never run by the authoring agent.

## Why now

Harness Kit's v2 consolidation (its backlog 103/104, June 2026) drew the
Mode A / Mode B boundary: ad-hoc judgment stays in harness-kit; event-driven
workflows live here. Code review is the first workload because Cerberus
already proved the appetite and Cloudflare published the playbook that
fixes what killed Cerberus (parallel reviewers, no coordinator, no cost
control — expensive and noisy).

## Design constraints (from blog.cloudflare.com/ai-code-review + IndyDevDan
## breakdown clipped in daybook)

- **Coordinator as filter:** sub-reviewers emit structured findings; the
  coordinator dedupes, kills nitpicks/speculation, verifies uncertain
  findings by reading source, posts ONE review. Bias toward approval;
  break-glass human override.
- **Tokenomics target: $1–2/review median.** Tiered model stack
  (state-of-the-art / workhorse / lightweight), risk-tiered compute
  (trivial <10 lines → coordinator + 1 generalist; full bench only for
  >100 lines / >50 files), shared context file + per-domain patch reads,
  aggressive prompt caching.
- **Resilience:** JSONL streaming output, step-finish retry/fallback
  chains, error classification, incremental re-review with prior context,
  "won't fix / disagree" dialogue on findings.
- **Reviewer prompts name what to IGNORE** per specialty, not just what to
  find.
- **Local-first trigger:** runnable from a terminal on any repo; the
  GitHub/GitLab webhook is one trigger among several.
- Reuse thinktank (thin Pi bench launcher) for the reviewer fan-out layer
  where it fits; substrate choice (Pi vs OpenCode vs Goose) goes through a
  Daedalus-style eval, not vibes — Cloudflare chose OpenCode for open
  source + programmatic-session SDK + familiarity; run our equivalent.

## Shared contracts

Read/write harness-kit's `meta/CONTRACTS.md`: lane cards, receipts,
backlog trailers, evidence paths.

## Oracle

- [ ] One real PR reviewed end-to-end: trigger → tiered reviewers →
      coordinator filter → single structured review posted → receipt on
      disk.
- [ ] Cost per review measured and reported for a medium diff.
- [ ] Same workload invoked locally with no webhook.
