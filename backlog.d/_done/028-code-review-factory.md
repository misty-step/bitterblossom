# Build the event-driven code review factory (absorbed Cerberus mission)

Priority: P1
Status: done
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
- **Candidate engine: harness-native headless orchestration.** Claude Code
  dynamic workflows run in `claude -p` / Agent SDK (GA June 2026): one
  saved workflow script can be the entire fan-out + adversarial
  cross-check + coordinator-filter, resumable, with per-stage model
  routing. Evaluate it against a hand-built coordinator before writing
  one — but it's Claude-only, so it competes as one engine, not the
  architecture.
- **Multi-harness is the plane's contract.** The factory's runner layer
  must outfit lanes with arbitrary harnesses — Claude, Codex, Pi,
  OpenCode, Droid, Antigravity, Grok, Cursor — so reviewer diversity is a
  config choice, not a rewrite. Harness-kit's `/roster` skill documents
  the headless invocation for each.

## Shared contracts

Read/write harness-kit's `meta/CONTRACTS.md`: lane cards, receipts,
backlog trailers, evidence paths.

## Oracle

- [x] One real PR reviewed end-to-end: misty-step/bitterblossom#843 —
      manual trigger → claude coordinator spawned reviewer subagents
      (correctness/security/simplification, each with ignore-lists) →
      coordinator filter → ONE structured review posted per run →
      receipts in plane/.bb/runs/<id>/ + ledger rows (runs 6b1b89ccc2d6,
      bcd8f5ee8552). The reviews caught every planted flaw plus a real
      cross-file NULL-cost crash verified against src/ledger.rs.
- [x] Cost per review measured: $2.46 small diff (12 lines), $3.09
      medium diff (140 lines / 2 files, ~228s). The $3.09 run breached
      the advisory max_cost_per_run and parked the task — the budget
      tier worked in production. Above the $1–2 target: tokenomics
      tuning spun out to ticket 034.
- [x] Same workload invoked locally with no webhook:
      `bb --config plane run review --payload '{"repo":"o/r","pr":N}'`
      — the webhook trigger (POST /hooks/review, HMAC, dedupe on
      /pull_request/head/sha) is declared in the same task.toml.

## Shipped shape (2026-06-10)

Pure config on the 031 spine: `plane/tasks/review/{card.md,task.toml}` +
`plane/agents/review-coordinator.toml`. The coordinator-as-filter,
risk-tiered fan-out, and ignore-lists live in the lane card; the engine
is claude headless (native subagent fan-out) — competing engines remain
swappable via the agent binding. Spine grew two generic features for it:
trigger payload materialized as EVENT.json, and `bb runs cancel`.
