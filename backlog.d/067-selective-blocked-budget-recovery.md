# Add selective recovery for blocked-budget runs

Priority: P1 | Status: ready | Estimate: S

## Goal

Let operators recover one blocked-budget run, or explicitly retire stale
blocked-budget runs, without unblocking every run parked behind the same task.

## Oracle

- [ ] Operators can list blocked-budget rows by task with created time,
      idempotency key, and state reason.
- [ ] Operators can release one blocked-budget run back to `pending` without
      unblocking every row for that task.
- [ ] Operators can mark a stale blocked-budget run as intentionally retired
      with a reason, preserving the ledger history.
- [ ] `bb task unpark <task>` remains available for deliberate bulk recovery
      and prints the number of rows it will release.
- [ ] Tests cover selective release, retirement, and the existing bulk unpark
      behavior.
- [ ] `./scripts/verify.sh` passes.

## Notes

Dogfood source: PR #860 opened at `2026-06-17T04:42:25Z` and GitHub delivery
`3826110084665049000` received `202 Accepted`, but production run
`fb16690dd35b` was recorded as `blocked_budget` because `review` was already
parked from a prior `$0.9708` run over `max_cost_per_run_usd = 0.50`.

Production had 20 blocked review rows. Running `bb task unpark review` would
release all of them, which could create stale review comments and make recovery
riskier than the original failure. The safe operator action needs run-level
release/retire, not only task-level unpark.
