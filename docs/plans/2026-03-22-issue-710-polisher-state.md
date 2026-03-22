# Plan: Issue #710 - Polisher Redispatch Guard

## Problem

Polisher keeps redispatching to PRs that were already polished because the conductor
does not persist whether a PR was polished after its most recent substantive change.

## Acceptance Slice

- persist `last_substantive_change_at` and `polished_at` per PR
- skip polish when `polished_at >= last_substantive_change_at`
- re-enable polish when commits or review discussion advance substantive activity
- cover persistence and malformed timestamp fallback paths with tests

## Invariants

- the conductor still decides eligibility; the worker does not
- transient GitHub lookup failures should not re-open already polished PRs with stored state
- no quality gate or review policy is weakened to satisfy CI

## Steps

- [x] Read task, repo instructions, and touched modules
- [ ] Refactor polisher eligibility flow into explicit fetch, persist, compare steps
- [ ] Add missing tests for malformed timestamps and PR state validation
- [ ] Run focused conductor tests and then broader verification
- [ ] Push the branch and re-check review feedback
