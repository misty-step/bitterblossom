# 039 submission loop — live evidence record (2026-06-11)

Ticket: `backlog.d/039-submission-loop.md` · Packet:
`docs/plans/2026-06-11-039-submission-loop.md`. All ledger rows live in
`plane/.bb/plane.db`; run artifacts under `plane/.bb/runs/<id>/`.

## The clean loop (oracle: blocked → fixed → clear, ≤ ~$1)

Change `test/039-live-drill`; plantings on rev `240a0088` (secrets
eprintln in local.rs + broken tail() in dispatch.rs — compile-, clippy-,
and test-green, catchable only by review); fix = rev `363ad5ea` (the
feat/039-submission-loop deliverable itself).

- **Round 1, submission `5ef86976cb0e`: `blocked`** — all four LLM
  members returned `blocking`, naming both plantings (fingerprints
  `a9aa358a…`, `55914b9b…`, `a8d25bb0…`, …). verify (command harness,
  real `./scripts/verify.sh` on sprite bb-polisher-2) passed, proving
  the plantings were invisible to the mechanical gate. Round cost
  $0.4169.
- **Round 2, submission `e93c0e383d76`: `clear`** — plane-owned round
  increment; `prior_report_json` snapshot of the round-1 report verified
  in SQLite and materialized as `REPORT.json` on the sprite workspace.
  All five members pass on the fixed rev. Round cost $0.5843.
- Concurrency: overlapping attempt timestamps across three sprites
  (lane-1, bb-polisher-2, bb-polisher-3), e.g. 21:29:29Z starts on all
  three hosts in the earlier chain.
- Members: verify=command, correctness=deepseek-v4-pro,
  security=deepseek-v4-pro, simplification=deepseek-v4-flash,
  product=grok-4.3, arbiter=kimi-k2-thinking.

## Live arbiter drill

On the blocked round-1 submission: `bb submit reject a8d25bb0… --reason
"doc-comment mismatch, not a runtime failure"` → gate re-eval kept the
finding **blocking** (rejection alone never unblocks). Live arbiter run
`7cc85186a09f` (kimi-k2-thinking) read the code and **overruled**:
"This is a legitimate blocking finding" — the finding stayed blocking.
The sustain direction is covered by the dev-plane drill and
`tests/submission.rs::rejected_blocking_finding_blocks_until_arbiter_sustains`.

## Termination + infra drills (dev plane, stub harnesses)

Blockers persisted three rounds → `escalated` exactly at
`max_rounds=3`, one `submission_escalated` notify. Dead-lettered
required member → `escalated`, never eternal pending. Arbiter CLI drill:
reject → still blocked → stub sustain → fingerprint moves to rejected.
`/api/gate` QA'd with/without `BB_API_TOKEN` (401 unauthorized).

## What the live rounds caught and fixed (system feedback working)

1. Reviewers found plantings but severity-shied (`minor`/`serious`) →
   cards now pin a never-downgrade blocking class; next rounds blocked
   correctly.
2. deepseek quoted `{env:?}` in prose → verdict JSON extraction crashed
   → parser scans candidate braces (regression test from live output).
3. Budget containment parked correctness at $0.39 vs $0.25 advisory cap
   (working as designed); unpark exposed that `bb run` stranded pending
   duplicates → converge fix.
4. Co-hosted storm members dead-lettered after 60s lease wait → lease
   wait now honors the predecessor's timeout.
5. kimi-k2.6 tail variance (16→45+ min on the same diff) → correctness
   rebound to deepseek-v4-pro (4 min, equally sharp on the plantings).
6. Required-member infra failures escalated loudly every time — no
   eternal pending was ever observed.
