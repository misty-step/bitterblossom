# Factory Audit Report #2

## Summary

- Date: 2026-03-16 (second audit, same day)
- Target Issue: #702 (not reached — queued behind higher-priority issues)
- Runs Observed: #680 (2 attempts, both blocked), #681 (failed), #711 (orphaned)
- PRs: #697 (existing, CI fixed by fixer), #713 (new, polished, not merged)
- Terminal State: **No merges.** Governance timing and artifact protocol blocked all runs.

## Timeline

| Time (UTC) | Event | Notes |
|---|---|---|
| 21:16:47 | Conductor start | 3 sprites healthy |
| 21:16:49 | Fixer dispatches to PR #697 | Red CI on existing PR |
| 21:17:03 | run-680 starts (adopted PR #697) | — |
| 21:17:04 | run-681 starts | Both to bb-builder |
| 21:25:22 | Fixer completes PR #697 | 8.5 min |
| 21:26:14 | PR #713 opened (run-681) | — |
| 21:30:29 | run-680 builder complete | **Blocked**: CI not attached yet |
| 21:31:10 | run-681 builder complete | **Failed**: artifact_missing |
| 21:31:15 | run-680 attempt #2 starts | Immediate retry |
| 21:34:57 | Polisher dispatches to PR #713 | — |
| 21:40:46 | run-680 attempt #2 complete | **Blocked**: Cerberus reviews in progress |
| 21:41:10 | run-711 starts | Slot opened |
| 21:49:59 | Polisher completes PR #713 | 15 min |
| 21:51:54 | **Rate limit exceeded** | All governance checks fail |
| ~21:52 | Conductor stopped | — |

## Findings

### Finding A: CI timing blocks governance (strengthens #681)

- Severity: P1
- Existing issue: #681 (commented with new evidence)
- Observed: run-680 blocked twice — first because CI checks hadn't attached, then because Cerberus reviews were still running
- Why it matters: Every PR with external reviews hits this. It's the #1 blocker for 24/7 operation.

### Finding B: Self-update git pull fails continuously (#716)

- Severity: P2
- Issue: #716
- Observed: `[self-update] git pull failed` every 13 seconds throughout entire session
- Root cause: Worktree branches diverge from master; no pull strategy configured
- Why it matters: Sprites never self-update during runs; log noise is massive

### Finding C: GitHub API rate limit exhausted (#717)

- Severity: P2
- Issue: #717
- Observed: Rate limit hit at 21:51 after ~35 min. All label/governance checks failed.
- Root cause: Per-issue API calls every poll cycle with 10+ open issues
- Why it matters: Conductor cannot sustain 24/7 operation at current API call rate

### Finding D: Artifact protocol failure (confirms #675)

- Severity: P1
- Existing issue: #675 (retro commented)
- Observed: run-681 builder opened PR #713 with passing CI but didn't write `builder-result.json`
- Why it matters: Codex harness doesn't produce the artifact. Every Codex-dispatched run risks this failure.

### Finding E: Retro auto-filed useful issue (#714)

- Severity: Positive
- Observed: Retro analyzed run-681 failure and filed #714 "No builder timeout or progress monitoring"
- Why it matters: The learning loop is working — retro converts failures into backlog items automatically

### Finding F: Stale leases from killed conductor sessions

- Severity: P2
- Existing issue: related to #676
- Observed: runs #675 and #676 had unreleased leases from previous killed conductor. These issues were permanently blocked until lease expiration.
- Why it matters: Operator killing the conductor creates orphaned leases that block future runs

## Backlog Actions

- New issues: #716 (self-update), #717 (rate limit)
- Commented: #681 (CI timing — stronger evidence)
- Retro auto-filed: #714 (builder timeout)
- Retro auto-commented: #675, #681, #707

## Reflection

- What Bitterblossom did well:
  - Fixer autonomously fixed CI on PR #697 (8.5 min)
  - Polisher completed review of PR #713 (15 min)
  - Retro loop working — auto-filed #714, commented on 3 existing issues
  - Shaper shaped 5 newly-filed issues automatically
  - Issue selection and concurrency limits work correctly

- What failed:
  - Zero merges in 35 minutes. Governance timing (#681) is now the critical path.
  - Artifact protocol (#675) kills Codex runs. Must be fixed for Codex harness.
  - Rate limit (#717) makes sustained operation impossible.

- What should be different next audit:
  - Fix #681 (wait for CI/reviews before governance check) — this alone would have let run-680 merge
  - Fix #675 (detect PR from branch, not artifact file) — this would have let run-681 proceed
  - Fix #717 (cache + batch API calls) — this would prevent operational collapse

- Delta from audit #1:
  - Audit #1 merged successfully (19 min). Audit #2 merged nothing.
  - The difference: audit #1 had all-green CI before governance checked. Audit #2 hit the timing race on every run.
  - The factory is deterministic when CI is fast. It breaks when external reviews take >30 seconds.
