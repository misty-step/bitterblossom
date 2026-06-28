# Adopt the Misty Step comic-ops aesthetic baseline

Priority: P2 · Status: pending · Estimate: M

## Goal
Evaluate and adopt the noir-ledger comic-ops flavor for Bitterblossom dispatch,
run ledger, readiness, and receipt surfaces.

## Oracle
- [ ] `DESIGN.md` or project docs name the chosen flavor, likely
      `noir-ledger`, plus any plane-specific component constraints.
- [ ] A representative dispatch/readiness/receipt surface is rendered or mocked
      with ledgers, proof strips, caption bands, and hard square panels.
- [ ] Aesthetic changes do not hide failure states, run costs, substrate
      readiness, or audit receipts.
- [ ] The implementation uses `@misty-step/aesthetic` commit `9bbe0f9` or later,
      or records a deliberate no-adoption decision.
- [ ] `./scripts/verify.sh` passes after implementation.

## Notes
Reference board:
`http://serenity.tail5f5eb4.ts.net:8788/bitterblossom-noir-ledger-concept.png`.
