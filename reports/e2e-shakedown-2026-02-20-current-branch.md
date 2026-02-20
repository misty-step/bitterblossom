# E2E Shakedown Report

## Summary

| Field | Value |
|-------|-------|
| Date | 2026-02-20 |
| Sprite | e2e-0220084402 (final run), e2e-0220083907 (prior validation run) |
| Issue | #405 — fix: improve expired token error message |
| Total Duration | 12m 43s (final run: 14:44:01Z → 14:56:44Z) |
| Overall Grade | D |
| Findings Count | P0: 1 / P1: 1 / P2: 1 / P3: 0 |

## Phase Results

| Phase | Score | Duration | Notes |
|-------|-------|----------|-------|
| 1. Build | PASS | <1s | `go build -o ./bin/bb ./cmd/bb` succeeded |
| 2. Fleet Health | FRICTION | ~2s | Works, but always attempts stale `FLY_API_TOKEN` exchange before fallback |
| 3. Issue Selection | PASS | <1s | `gh issue view 405` returned full body |
| 4. Credential Validation | PASS | ~2s | Correctly failed as not configured; auth resolution succeeded |
| 5. Dispatch | FAIL | ~12m | Completed task/PR detection, then entered unstable post-completion wait path |
| 6. Monitor | FAIL | >5m silent gap | Off-rails warning/abort fired during PR-check wait |
| 7. Completion | FAIL | n/a | Manual termination required (wrapper exit 143) |
| 8. PR Quality | FRICTION | n/a | Existing PR branch collision caused heavy git conflict recovery path |

## Findings

### Finding 1: Stale Branch State Causes Git Conflict Spiral

| Field | Value |
|-------|-------|
| Category | F4: Stale State |
| Severity | P0 |
| Phase | 5 / 8 |
| Known Issue | #425 (NEW) |

**Observed:** Agent reused an existing remote branch, hit non-fast-forward push, attempted rebase recovery, was blocked by repo rebase policy, entered detached/unmerged state, and performed low-level `.git` recovery steps.

**Expected:** Branch collision should be handled safely (unique branch, safe merge strategy, or fast fail) without rebase-only recovery and without `.git` surgery.

**Impact:** End-to-end loop reliability breaks under stale branch state; requires manual intervention.

**Recommended Fix:** Make Ralph git strategy branch-collision-safe and policy-aware; forbid rebase-only recovery paths.

---

### Finding 2: Off-Rails Fires During PR-Check Wait and Does Not End Run Promptly

| Field | Value |
|-------|-------|
| Category | F6: Timing & Performance |
| Severity | P1 |
| Phase | 6 / 7 |
| Known Issue | #416 (commented), #293 (commented) |

**Observed:** After `TASK_COMPLETE` + valid PR detection, run entered PR-check waiting with long silence. Off-rails logged abort after 5m25s, but run still needed manual kill.

**Expected:** Either emit heartbeat progress while waiting, or terminate promptly/cleanly when off-rails triggers.

**Impact:** Operator sees contradictory state ("aborting" but still running), reducing trust and forcing manual cleanup.

**Recommended Fix:** Stop off-rails after `ralphCmd.Run()` and emit periodic PR-check pending progress; ensure cancellation propagates through PR-check wait path.

---

### Finding 3: Credential Fallback Works but Produces Confusing Double-Exchange Noise

| Field | Value |
|-------|-------|
| Category | F2: Confusing Output |
| Severity | P2 |
| Phase | 2 / 4 |
| Known Issue | #422 (commented) |

**Observed:** Every command logs both exchange attempts:
1) `source=FLY_API_TOKEN` (unauthorized)
2) `source=fly auth token` (success)

**Expected:** Fallback should be transparent or clearly summarized once, without repetitive failure noise each invocation.

**Impact:** Operators may misread runs as failed despite successful fallback.

**Recommended Fix:** Cache/short-circuit stale env token after first unauthorized or suppress expected fallback noise.

## Regression Check

| Issue | Description | Status |
|-------|-------------|--------|
| #277 | Stale TASK_COMPLETE | UNCHANGED |
| #293 | --wait polling loops | REGRESSED (comment added with 2026-02-20 evidence) |
| #294 | Oneshot exits with zero effect | UNCHANGED |
| #296 | Proxy health check no diagnostics | UNCHANGED |
| #320 | Stdout pollution breaks JSON | UNCHANGED |

## Timeline

| Time | Event |
|------|-------|
| 14:44:01 | Phase 1 start — build |
| 14:44:02 | Phase 2 start — create fresh sprite e2e-0220084402 |
| 14:44:19 | Phase 4 start — credential validation dispatch |
| 14:44:27 | Setup start |
| 14:44:41 | Setup complete |
| 14:44:46 | Phase 5 dispatch start |
| ~14:51 | `TASK_COMPLETE` + PR detection summary printed |
| ~14:56 | Off-rails warning then abort message during PR-check wait |
| 14:56:44 | Manual termination; wrapper exit 143 |

## Raw Output

<details>
<summary>Dispatch output tail (final run)</summary>

```text
... (see full log at /tmp/e2e7/phase5_dispatch.out)
dispatch outcome: task_complete=true blocked=false branch="fix/expired-token-hint" dirty_files=1 commits_ahead=1 open_prs=1 pr_number=417 pr_query=ok
[off-rails] no output for 55s (abort in 4m5s)
[off-rails] aborting: no output for 5m25s (threshold 5m0s)
```

</details>

## Recommendations

### Immediate (P0-P1)

- Resolve #425 (stale-branch collision handling) before further autonomous dispatch reliance.
- Resolve #416/#293 behavior in post-ralph PR-check waiting path.

### Next Sprint (P2)

- Reduce credential fallback noise and improve auth-resolution messaging clarity (#422).

### Backlog (P3)

- n/a
