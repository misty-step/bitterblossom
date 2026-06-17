# Extract the review-storm gate protocol out of the spine

Priority: P1 | Status: ready | Estimate: M

## Goal

Shrink the spine by moving `src/submit.rs`'s review-specific gate judgment
(verdict vocabulary, finding/severity shapes, fingerprint dedup, rejection /
arbiter logic, `evaluate`) out of `src/` into workload-config-driven
evaluation, keeping only the generic submission state machine as mechanism —
freeing the LOC headroom that 064 and 053 need.

## Decision (resolved 2026-06-17)

The question this ticket originally asked — mechanism or workload? — is
**answered: workload.** Evidence (groom budget lane, read against live code):

- Hardcoded review vocab: `VERDICTS = pass|blocking|advisory` (`submit.rs:9`),
  `SEVERITIES = blocking|serious|minor` (`submit.rs:10`).
- `Finding{severity,file,line,claim,evidence,fingerprint}` is a code-review
  finding shape (`submit.rs:26-38`).
- `arbiter_sustains` (`submit.rs:299`), `reject_finding`/`rejections`
  (`submit.rs:280-298`), `enforce_fingerprints` (`submit.rs:352`), and
  `evaluate`'s blocking-vs-advisory + arbiter-override logic
  (`submit.rs:411-539`) are review-storm policy, not generic gating.
- Generic mechanism is thin: the submission state machine + `record_verdict`.

So ~300–400 of 510 lines are workload judgment living in the core.

## Oracle

- [x] Written decision: review-workload judgment, not a general plane primitive.
- [ ] Verdict vocabulary, severity semantics, fingerprinting, rejection/arbiter
      logic, and `evaluate` are driven by workload config (e.g. a `[gate]` spec
      block + generic finding JSON), not hardcoded in `src/`.
- [ ] The live review storm passes the full submission-loop drill (both rounds,
      arbiter, termination) unchanged after extraction — behavior preserved.
- [ ] `dispatch.rs`/`main.rs` no longer construct review-shaped verdict types
      inline (today: `dispatch.rs:286-394`, `main.rs:466-482`).
- [ ] The spine LOC cap drops to reflect the extraction (re-baseline DOWN).
- [ ] `./scripts/verify.sh` passes.

## Verification System

- Claim: the review gate behaves identically with its judgment in config, not
  `src/`.
- Falsifier: any submission-loop drill verdict differs (a blocker passes, an
  arbiter override flips, a fingerprint stops deduping).
- Driver: the submission-loop + termination + arbiter drills (CLAUDE.md), dev
  plane + stub harnesses, run before and after extraction.
- Grader: identical gate JSON both rounds pre/post; `src` LOC drops by ~300+.
- Evidence packet: gate JSON diffs (expect none) + before/after LOC + the moved
  config.
- Cadence: once, as a dedicated refactor lane (route through `/shape` then
  `/refactor`).

## Notes

This is the **real budget unlock**, not a quick win. The decision is cheap and
now made; the extraction is a genuine refactor because `dispatch.rs` and
`main.rs` build submit's review-shaped types inline (deep coupling, not a clean
call boundary) — re-homing that construction is the bulk of the work. Sequence
(groom 2026-06-17): ship the zero/low-budget capability work first (070 gate-fix
reflex, 071 refactor workload, 072 observability core API), then do this
extraction as its own lane, then 064 (+~70 LOC) and 053 (+~120-250 LOC) land
with headroom. A conscious re-baseline to ~5300 is the honest fallback for 064
alone if this slips; 053 still needs the headroom this frees.
