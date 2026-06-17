# Emit a builder fix-prompt packet when a gate blocks

Priority: P1 | Status: ready | Estimate: S

## Goal

When a submission gate returns `blocked`, a reflex turns the blocking findings
into a bounded builder packet (a suggested `bb run build`/`refactor` input) —
without editing code, resolving runs, parking tasks, or merging.

## Oracle

- [ ] A `gate.blocked` event (or manual `bb run fix-prompt --submission <id>`)
      produces a packet artifact naming each blocking fingerprint, its
      file/line, the claimed defect, and a suggested follow-up `bb run` command.
- [ ] The reflex never mutates code, runs, parks, or merges — report-only, the
      same red lines as `ci-diagnose`.
- [ ] Defined as a task/agent/card + payload contract; no review judgment lands
      in `src/`. Any spine cost is trigger/notify wiring only (≤15 LOC) — fits
      the current budget slack.
- [ ] Manual dogfood payload + report-artifact shape test + a `bb` receipt
      (run id, cost).
- [ ] `./scripts/verify.sh` passes.

## Verification System

- Claim: a blocked gate yields an actionable, bounded fix packet with zero side
  effects.
- Falsifier: the reflex edits code/merges, or emits a packet missing a blocking
  fingerprint.
- Driver: dev plane + stub harness; seed a blocking gate (reuse the
  submission-loop drill), fire the reflex.
- Grader: packet names every planted blocker + a suggested next run; no ledger
  write beyond the report row.
- Evidence packet: packet artifact + gate JSON + `bb runs show` receipt under
  the repo evidence path.
- Cadence: the submission-loop drill in CLAUDE.md, extended with the fix-prompt
  step.

## Notes

Graduated from epic 062 (child 1) so the cheapest, highest-leverage reflex is
pickup-ready. Budget: the `gate.blocked` decision already exists
(`src/submit.rs` `evaluate` + the notify at `submit.rs:527-536`), so this is
mostly config/cards. Completes the SDLC loop: review/CI gate → fix packet →
`bb run build`/`refactor`. Boundary (061/062): the spine routes/leases/budgets/
records; the fix-packet meaning lives in the card + agent config + payload
contract.
