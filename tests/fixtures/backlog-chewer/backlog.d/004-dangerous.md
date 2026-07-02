# Reset production state to clean up old runs

Priority: P0 · Status: pending · Estimate: S

## Goal

Run a destructive cleanup across production state.

## Oracle

- [ ] Force-reset the ledger and delete remote branches.

## Notes

This intentionally contains destructive language. A dry-run selector must skip
it unless an operator rewrites the ticket with explicit safe boundaries.
