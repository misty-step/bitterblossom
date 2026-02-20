# E2E Shakedown Report

## Summary

| Field | Value |
|-------|-------|
| Date | 2026-02-20 |
| Sprite | `e2e-0220033237` (freshly created) |
| Issue | #407 - feat: add --dry-run flag to dispatch |
| Total Duration | ~4m47s (03:32:51-03:37:38 UTC) |
| Overall Grade | C |
| Findings Count | P0: 0 / P1: 2 / P2: 1 / P3: 0 |

## Phase Results

| Phase | Score | Duration | Notes |
|-------|-------|----------|-------|
| 1. Build | PASS | ~1s | `go build -o ./bin/bb ./cmd/bb` succeeded, help output valid |
| 2. Fleet Health | PASS | ~20s | Fleet status returned; fresh sprite reachable and `warm` |
| 3. Issue Selection | PASS | ~1s | `gh issue view 407` returned full body; context embedded verbatim |
| 4. Credential Validation | PASS | ~45s | Smoke dispatch wrote `TASK_COMPLETE` cleanly |
| 5. Dispatch | PASS | ~2m53s | Real run produced commit + PR #421 + TASK_COMPLETE |
| 6. Monitor | PASS | ~2m53s | Continuous stream output; no >60s observed silence |
| 7. Completion | FAIL | immediate | Exit was success, but outcome summary contradicted reality (`open_prs=0` despite open PR) |
| 8. PR Quality | FRICTION | immediate | PR created correctly; checks were still pending when dispatch returned success |

## Findings

### Finding 1: Credential bootstrap blocked on Fly token exchange despite valid sprite auth

| Field | Value |
|-------|-------|
| Category | F5: Credential Pain |
| Severity | P1 |
| Phase | Pre-flight |
| Known Issue | #422 (NEW) |

**Observed:** `bb status` failed with unauthorized Fly token exchange, while `sprite api /v1/sprites` succeeded using local sprite auth.

**Expected:** `bb` should have a first-class fallback path when Fly exchange fails but sprite auth is present.

**Impact:** e2e was blocked until manual token extraction workaround.

**Recommended Fix:** Add sprite-auth fallback (or explicit supported recovery path) in `spriteToken` flow.

---

### Finding 2: Dispatch outcome still reports `open_prs=0` with an open PR

| Field | Value |
|-------|-------|
| Category | F2: Confusing Output |
| Severity | P2 |
| Phase | 7 |
| Known Issue | #419 (commented) |

**Observed:** Completion printed `open_prs=0 pr_number=0` while `work produced` listed PR #421.

**Expected:** Outcome counters should match produced artifacts.

**Impact:** Summary cannot be trusted by operators or automation.

**Recommended Fix:** Ensure outcome script runs gh queries with auth (`GH_TOKEN`) and fail loudly on query errors.

---

### Finding 3: Green-check completion gate bypassed due zero PR detection

| Field | Value |
|-------|-------|
| Category | F1: Silent Failure |
| Severity | P1 |
| Phase | 7-8 |
| Known Issue | #420 (commented) |

**Observed:** Dispatch returned success before PR checks were green because PR detection reported zero open PRs and skipped the gate.

**Expected:** With `--require-green-pr=true`, success should wait for/require green PR checks (or fail if unknown).

**Impact:** False-positive completion for issue-to-merge automation.

**Recommended Fix:** Make PR detection robust and treat unknown PR-check state as non-success.

## Regression Check

Known issues from previous shakedowns. Mark each as REGRESSED, FIXED, or UNCHANGED.

| Issue | Description | Status |
|-------|-------------|--------|
| #277 | Stale TASK_COMPLETE | FIXED |
| #293 | --wait polling loops | FIXED |
| #294 | Oneshot exits with zero effect | FIXED |
| #296 | Proxy health check no diagnostics | UNCHANGED (not exercised in this run) |
| #320 | Stdout pollution breaks JSON | FIXED in exercised path (`bb logs --json` output parsed cleanly) |

## Timeline

| Time (UTC) | Event |
|------------|-------|
| 03:32:51 | Fresh sprite `e2e-0220033237` created |
| 03:33:04 | `bb setup` succeeded with persona fallback to `sprites/bramble.md` |
| 03:33:34 | Fleet status confirmed sprite `warm` + reachable |
| 03:33:59 | Credential validation dispatch completed (`TASK_COMPLETE`) |
| 03:34:15 | Real dispatch started for issue #407 |
| 03:36:44 | PR #421 created |
| 03:37:08 | Dispatch completed (`TASK_COMPLETE`) with contradictory `open_prs=0` |
| 03:37:24 | `Go Checks` passed for PR #421 while other checks remained pending |

## Raw Output

<details>
<summary>Dispatch completion tail</summary>

```text
=== work produced ===
--- commits ---
6f8795d feat: add --dry-run flag to bb dispatch (#407)
--- PRs ---
[{"title":"feat: add --dry-run flag to bb dispatch","url":"https://github.com/misty-step/bitterblossom/pull/421"}, ...]
dispatch outcome: task_complete=true blocked=false branch="feat/dispatch-dry-run" dirty_files=2 commits_ahead=1 open_prs=0 pr_number=0

=== task completed: TASK_COMPLETE signal found ===
```

Full transcript: `/tmp/e2e4/phase5_dispatch_407.txt`

</details>

## Recommendations

### Immediate (P0-P1)

- Fix auth fallback when Fly exchange is unauthorized but sprite auth is available (#422).
- Fix PR detection/auth in outcome script so green-check gating cannot be bypassed (#419, #420).

### Next Sprint (P2)

- Add hard assertion tests for `inspectDispatchOutcome` gh auth path and PR counter consistency (#419).

### Backlog (P3)

- Add explicit e2e assertion for completion semantics under pending vs failing vs passing PR checks.
