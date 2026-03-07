# PLAN: Issue #478 — Prevent blocked issues from immediate retry in backlog loop

## Problem

When `run_once` exits with `rc=2` (blocked), the `finally` block calls
`release_lease`, which frees the issue for immediate re-pick on the next poll.
A blocked issue gets re-leased, re-run, and spams repeated blocker comments.

## Root Cause

`release_lease` is called unconditionally in `run_once`'s `finally` block.
Blocked runs need their lease kept active (not released) until an operator
explicitly re-queues the issue.

## Solution

Treat `blocked` as a scheduler state in the lease table:

1. Add `blocked_at` column to `leases`.
2. New `block_lease()` sets `blocked_at = now, lease_expires_at = null`.
3. `run_once` tracks `block_on_release = True` at each `return 2` point,
   and the `finally` calls `block_lease` instead of `release_lease`.
4. `pick_issue` already skips issues where `released_at is null` — so blocked
   issues (with `released_at = null`) are excluded automatically.
5. `acquire_lease` already returns False for active leases — blocked leases
   have `released_at = null`, `lease_expires_at = null`, so they block re-acquisition.
6. `reap_expired_leases` skips `lease_expires_at = null` leases — no change needed.
7. New `requeue-issue` command clears `blocked_at`, sets `released_at = now`.

## Affected Files

- `scripts/conductor.py`
- `scripts/test_conductor.py`
- `docs/CONDUCTOR.md`

## Steps

- [x] Plan written and verified
- [x] Create branch `factory/478-p1-conductor-prevent-blocked-iss-1772842172`
- [ ] `init_db`: add `ensure_column(conn, "leases", "blocked_at", "text")`
- [ ] Add `block_lease(conn, repo, issue_number)` function
- [ ] Modify `run_once`: track `block_on_release`, conditional finally
- [ ] Add `requeue_issue(args)` function
- [ ] Add `requeue-issue` subparser
- [ ] Write tests (AC1: not re-picked, AC3: re-queue, unit tests for block_lease)
- [ ] Update `docs/CONDUCTOR.md`
- [ ] Run `python3 -m pytest -q scripts/test_conductor.py`
- [ ] Push branch, open draft PR
- [ ] Write builder artifact

## Review

(To be filled in after implementation)
