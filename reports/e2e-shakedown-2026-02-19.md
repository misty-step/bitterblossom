# E2E Shakedown Report

## Summary

| Field | Value |
|-------|-------|
| Date | 2026-02-19 |
| Sprite | bramble (fern used for dry-run check first) |
| Issue | #411 - fix: fleet status should surface busy sprites before dispatch |
| Total Duration | ~41m (07:48-08:29 CST) |
| Overall Grade | D |
| Findings Count | P0: 1 / P1: 3 / P2: 1 / P3: 1 |

## Phase Results

| Phase | Score | Duration | Notes |
|-------|-------|----------|-------|
| 1. Build | PASS | ~1s | `go build -o ./bin/bb ./cmd/bb` succeeded, binary usable |
| 2. Fleet Health | FRICTION | ~16s | Skill command failed (`status --format text` unknown); fallback `bb status` succeeded |
| 3. Issue Selection | PASS | ~1m | Issue #411 fetched, body embedded verbatim in prompt |
| 4. Credential Validation | FRICTION | ~10s | Dry-run hit active loop guard on fern (credentials resolved, but sprite busy) |
| 5. Dispatch | FAIL | ~39m across retries | Skill flags invalid (`--execute` unknown), first run stale (`model=default`), rerun eventually produced PR then ended with off-rails abort |
| 6. Monitor | FAIL | ~38m | Multiple >60s silence windows; monitor commands in skill (`--format`, `watchdog`) unavailable |
| 7. Completion | FAIL | final run 1671s | Exit code `4` after off-rails, despite commits + PR + green CI; no TASK_COMPLETE signal |
| 8. PR Quality | PASS | ~1m check | PR #414 open, correct base (`master`), clean commit history, CI green |

## Findings

### Finding 1: Dispatch continued past useful completion and re-entered loop

| Field | Value |
|-------|-------|
| Category | F4: Stale State |
| Severity | P0 |
| Phase | 6-7 |
| Known Issue | #293 (reopened) |

**Observed:** After PR work was already produced, run entered `iteration 2` instead of terminating, then stayed in CI polling/wait behavior until off-rails aborted.

**Expected:** Once completion criteria are met, dispatch should terminate cleanly instead of continuing iterations.

**Impact:** Long-running false-active dispatch behavior; operator cannot trust run completion boundaries.

**Recommended Fix:** Reconcile completion state immediately when useful completion is reached; avoid launching further iterations once run is effectively done.

---

### Finding 2: Off-rails false-failed a run that had valid output

| Field | Value |
|-------|-------|
| Category | F6: Timing & Performance |
| Severity | P1 |
| Phase | 6-7 |
| Known Issue | #415 (NEW) |

**Observed:** During `sleep 300 && gh pr checks 414`, output went silent and off-rails aborted at `no output for 5m28s (threshold 5m0s)`.

**Expected:** Legitimate long CI-wait states should emit heartbeat or be treated as active, not off-rails.

**Impact:** False failure after useful work was already produced.

**Recommended Fix:** Add heartbeat/progress during wait loops or make off-rails detector aware of known long-wait operations.

---

### Finding 3: TASK_COMPLETE missing after successful PR path

| Field | Value |
|-------|-------|
| Category | F1: Silent Failure |
| Severity | P1 |
| Phase | 7 |
| Known Issue | #285 (reopened) |

**Observed:** Post-run `bb status bramble` showed `TASK_COMPLETE: absent` with branch `fix/411-fleet-status-busy-sprites` and new commits present.

**Expected:** Completion signal should be written when run reaches successful task completion path.

**Impact:** Completion contract breaks; contributes to loop/exit ambiguity.

**Recommended Fix:** Enforce signal write as a hard invariant before considering run complete.

---

### Finding 4: Exit code did not reflect practical outcome

| Field | Value |
|-------|-------|
| Category | F1: Silent Failure |
| Severity | P1 |
| Phase | 7 |
| Known Issue | #298 (reopened) |

**Observed:** Dispatch exited `4`, but produced commits (`4096227`, `95a679b`, `0c12694`), PR #414, and green checks.

**Expected:** Exit status should distinguish operator-usable success from true failure paths.

**Impact:** Automation and humans interpret successful work as failed run.

**Recommended Fix:** Return outcome-aware exit codes, especially when artifacts and CI indicate success.

---

### Finding 5: Effective runtime exceeded user timeout expectation

| Field | Value |
|-------|-------|
| Category | F6: Timing & Performance |
| Severity | P2 |
| Phase | 7 |
| Known Issue | #406 (commented) |

**Observed:** Command used `--timeout 25m` but run duration was ~27m51s.

**Expected:** Timeout behavior should be intuitive/predictable for operators.

**Impact:** Hard to reason about when dispatch will stop in degraded states.

**Recommended Fix:** Make grace period proportional or configurable; surface effective timeout in startup output.

---

### Finding 6: e2e skill workflow drifted from current CLI

| Field | Value |
|-------|-------|
| Category | F7: Flag & CLI Ergonomics |
| Severity | P3 |
| Phase | 2,5,6 |
| Known Issue | #321 (related) |

**Observed:** Skill-specified commands failed in current CLI:
- `bb status --format text` (unknown flag)
- `bb dispatch ... --execute --wait` (unknown flag)
- `bb watchdog --sprite ...` (unknown command)

**Expected:** Shakedown skill and CLI should remain aligned.

**Impact:** Test workflow introduces avoidable manual recovery steps.

**Recommended Fix:** Update skill docs to current command surface (or restore compatibility aliases).

## Regression Check

Known issues from previous shakedowns. Mark each as REGRESSED, FIXED, or UNCHANGED.

| Issue | Description | Status |
|-------|-------------|--------|
| #277 | Stale TASK_COMPLETE | FIXED |
| #293 | --wait polling loops | REGRESSED |
| #294 | Oneshot exits with zero effect | FIXED |
| #296 | Proxy health check no diagnostics | UNCHANGED |
| #320 | Stdout pollution breaks JSON | UNCHANGED |

## Timeline

| Time | Event |
|------|-------|
| 07:48:49 | Phase 1 complete - build and help check |
| 07:48:53 | Phase 2 start - `bb status --format text` failed (`unknown flag`) |
| 07:49:09 | Fleet status fallback succeeded |
| 07:49:34 | Phase 3 complete - issue #411 fetched |
| 07:49:45 | Phase 4 dry-run on fern blocked: active dispatch loop |
| 07:50:14 | Phase 5 first dispatch command failed (`--execute` unknown) |
| 07:51:19 | First bramble dispatch started (`model=default`, stale setup) |
| 08:00:53 | `bb setup bramble` run to sync current branch behavior |
| 08:01:41 | `bb kill bramble` then rerun dispatch |
| 08:01:45 | Rerun iteration 1 started (`model=sonnet-4.6`) |
| 08:16:48 | Iteration 2 started (run did not terminate after PR path) |
| 08:29:33 | Off-rails abort and command exit code 4 |
| 08:30:19 | Post-run PR checks confirmed all green |

## Raw Output

<details>
<summary>Dispatch command and key output</summary>

```text
starting ralph loop (max 50 iterations, 25m0s timeout, harness=claude)...
[ralph] harness=claude model=sonnet-4.6
[ralph] iteration 1 / 50 ...
...
[ralph] iteration 2 / 50 ...
...
[off-rails] aborting: no output for 5m28s (threshold 5m0s)

=== work produced ===
--- commits ---
0c12694 fix: skip busy probe for unreachable sprites in fleet status
95a679b fix: use errors.As for exit code extraction in spriteBashRunnerForStatus
4096227 feat: surface busy sprites in fleet status
--- PRs ---
[{"title":"fix: fleet status surfaces busy sprites before dispatch","url":"https://github.com/misty-step/bitterblossom/pull/414"}, ...]

=== off-rails detected: off-rails: no output for 5m28s ===
```

Full transcript captured at `/tmp/e2e2_phase5d_dispatch_after_kill.txt`.

</details>

## Recommendations

### Immediate (P0-P1)

- Resolve completion-state loop regression (#293 reopened).
- Prevent false off-rails failures during long CI waits (#415).
- Enforce TASK_COMPLETE signaling in successful completion paths (#285 reopened).
- Align exit code semantics with practical outcome (#298 reopened).

### Next Sprint (P2)

- Fix timeout grace behavior so `--timeout` is predictable (#406).

### Backlog (P3)

- Align e2e skill command examples with current CLI surface (#321 related).
