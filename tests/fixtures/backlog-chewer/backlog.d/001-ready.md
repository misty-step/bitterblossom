# Add a receipt to the dry-run fixture

Priority: P2 · Status: ready · Estimate: S

## Goal

Add a receipt field to the fixture dry-run report so operators can see the
selected ticket, verifier, budget, and stop conditions in one artifact.

## Oracle

- [ ] `REPORT.json` includes `selected_ticket.id`, `verifier`, `budget`, and
      `stop_conditions`.
- [ ] The selected ticket names the expected changed paths.
- [ ] `cargo test --locked --test backlog_chewer_contract -- --nocapture`
      passes.
- [ ] `./scripts/verify.sh` passes.

## Notes

Small, local, no secrets, no external side effects.
