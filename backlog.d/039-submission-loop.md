# Build the submission loop: verdict storm, mechanical gate, bounded rounds

Priority: P1 · Status: ready · Estimate: L

## Goal

Completed agent work gets autonomously quality-assured and landed — a
storm of verdict tasks attacks the change, `bb gate` decides
mechanically, the implementing agent loops on the report, and
termination is guaranteed by the round cap and escalation — never by
silencing fresh blockers.
No human code-reading; no PR required for coordination.

## Oracle

- [ ] `./scripts/verify.sh` green including new submission-lifecycle,
      verdict-parsing, command-harness, and gate-rule tests (pending
      with run states / fresh blocker blocks any round / serious never
      blocks / arbiter-gated rejection of blocking findings / escalate
      at max_rounds and on dead required member / clear only over a
      complete round)
- [ ] Live: seeded-flaw branch goes blocked (round 1, plantings named)
      → fixed → clear (round 2) → squash-landed, ≥2 storm members
      concurrent on distinct sprites, total loop cost ≤ ~$1
- [ ] Live arbiter drill: rejected blocking finding stays blocking
      until an arbiter verdict sustains the rejection
- [ ] Termination drill (stub): persistent blockers → `escalated` at
      max_rounds with one notify; dead-lettered required member →
      `escalated`, never eternal pending
- [ ] Spine stays ≤ 5k LOC

## Notes

Context packet: `docs/plans/2026-06-11-039-submission-loop.md` (premise:
operator voice transcript, sanitized, hashed in packet). v1 is
dispatch-driven by ratified decision; reflex ingress (post-receive on a
self-hosted remote, GitHub App only if needed) is the follow-up once the
plane has a durable home. Verdict storage is ledger-canonical; git-notes
export and jj change-id keying are deliberately deferred (change keys
are opaque strings — the jj swap is config, not spine). Codex
adversarial critique (receipt 20260611T161652) drove a major revision:
submission sessions with CAS lifecycle, plane-owned rounds and report
snapshots, no demotion of fresh blockers, command harness for verify,
arbiter quorum for rejections.
