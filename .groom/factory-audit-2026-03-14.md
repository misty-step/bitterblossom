# Factory Audit Report

## Summary

- Date: 2026-03-14
- Run ID: run-625-1773502623
- Issue: #625 [P0] CI: add Elixir test/format/compile pipeline
- PR: #629 (OPEN, not merged — blocked by governance bug)
- Worker: noble-blue-serpent
- Reviewers: Cerberus (trusted external surface)
- Terminal State: FAILED (ci_timeout)

## Timeline

| Time | Event | Notes |
|------|-------|-------|
| 10:35:22 | First run attempt | Killed by pipe closure (head -5) |
| 10:35:22 | Lease orphaned | RunServer died without releasing lease |
| 10:36:09 | Second run attempt | Failed — lease still held |
| 10:37:00 | Manual lease release | sqlite3 UPDATE on leases table |
| 10:37:03 | Third run — lease acquired | Clean start |
| 10:37:04 | Workspace ready | 1.5s — warm mirror + fetch worked fast |
| 10:37:04 | Builder dispatched | |
| 10:48:34 | Builder completed | 11m 30s, PR #629 opened |
| 10:48:35 | Governance entered | 300s PR age wait |
| 10:53:35 | PR age wait done | CI polling begins |
| 10:53:36 | All CI checks green | Elixir 14s, merge-gate pass |
| 10:53:36-11:03:44 | CI poll loop | "not green yet" x ~20 iterations |
| 11:03:44 | CI timeout | Run marked FAILED |

Total wall clock: 26m 41s (should have been ~17m with successful merge)

## Findings

### Finding 1: Lease not released on process kill (CONFIRMED)
- Severity: P1
- Existing issue: None (confirmed domain audit prediction)
- Observed: First run killed by head pipe closure, lease remained in DB, blocking retry
- Expected: RunServer terminate callback should release lease
- Why it matters: Any OOM, crash, or operator interrupt orphans the lease
- Evidence: `sqlite3 .bb/conductor.db "SELECT * FROM leases WHERE issue_number = 625"`

### Finding 2: worktree_path not persisted during building
- Severity: P3
- Existing issue: None
- Observed: worktree exists on sprite but `show-runs` shows `worktree_path: null`
- Expected: Path should be set when workspace prep completes
- Why it matters: Operator can't inspect builder workspace from run ledger

### Finding 3: No progress visibility during build phase
- Severity: P2
- Existing issue: None
- Observed: 11.5 minutes of "dispatching builder" with no intermediate signals
- Expected: Turn count updates, phase progress, token estimates
- Why it matters: Operator can't distinguish "working" from "stuck"

### Finding 4: Builder violated issue boundaries
- Severity: P2 (issue-writing lesson, not code bug)
- Existing issue: #628 (prompt enrichment — related)
- Observed: Builder modified 8 conductor files despite boundary saying "Do NOT change conductor code"
- Expected: Builder respects stated boundaries
- Why it matters: Boundary enforcement depends on prompt quality, not mechanical gates

### Finding 5: Issue boundary contradicted its own AC
- Severity: P3 (process improvement)
- Existing issue: None
- Observed: "Don't modify conductor code" + "CI must pass" are contradictory when code has warnings
- Expected: AC should be achievable within boundaries
- Why it matters: Issue writing quality affects builder behavior

### Finding 6: checks_green? fails on null-conclusion entries (CRITICAL)
- Severity: P0
- New issue: #630
- Observed: All CI green but conductor polled "not green" for 10 minutes then timed out
- Expected: Null-conclusion entries (CodeRabbit) should be filtered, not treated as failures
- Why it matters: Blocks ALL merge operations — factory cannot complete any run
- Evidence: `gh pr view 629 --json statusCheckRollup` shows null entry

### Finding 7: docs/CONDUCTOR.md references only Python conductor
- Severity: P3
- Existing issue: #488 (architecture docs)
- Observed: 700-line doc references `python3 scripts/conductor.py` exclusively
- Expected: Should reference Elixir conductor or be marked as deprecated
- Why it matters: Operators following the docs will use the wrong system

## Backlog Actions

- **New issue filed:** #630 [P0] checks_green? fails on null-conclusion entries
- **Existing issue to comment:** #488 (docs/CONDUCTOR.md is stale — concrete evidence)
- **Existing issue confirmed:** Domain audit findings 1.2 (lease orphan), 3.1 (no Elixir CI)
- **No action needed:** Findings 2, 3, 5 are minor — can be folded into existing issues during next groom

## Reflection

- What Bitterblossom did well: Workspace prep is fast (1.5s), heartbeat works, builder delivered correct CI pipeline in 11.5 minutes, governance phases are logged clearly
- What felt brittle: Lease lifecycle (no cleanup on kill), CI check matching (one null breaks everything), builder boundary enforcement is prompt-only
- What should be simpler next time: The null-conclusion bug should have been caught by a test. Once #630 is fixed and #625 CI pipeline is merged, the factory should be able to merge autonomously.
