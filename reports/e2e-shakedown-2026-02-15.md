# E2E Shakedown Report

## Summary

| Field | Value |
|-------|-------|
| Date | 2026-02-15 |
| Sprite | fern (thorn attempted first, unreachable) |
| Issue | #352 — fleet status shows 'running' for idle sprite with no task |
| Total Duration | ~9 minutes (build-to-completion) |
| Overall Grade | C |
| Findings Count | P0: 0 / P1: 3 / P2: 3 / P3: 0 |

## Phase Results

| Phase | Score | Duration | Notes |
|-------|-------|----------|-------|
| 1. Build | PASS | 0.9s | Clean build, no warnings |
| 2. Fleet Health | FRICTION | 1.4s | 3 sprites warm but 2 unreachable; thorn idle/running contradiction |
| 3. Issue Selection | PASS | <5s | #352 fetched cleanly, context fits in prompt |
| 4. Credential Validation | PASS | <2s | Dry-run shows full plan including preflight probe |
| 5. Dispatch | FRICTION | - | thorn: preflight correctly rejected (unreachable). fern: pipeline completed in 5s to agent start |
| 6. Monitor | FRICTION | 8min | Zero progress output during agent execution; status contradicts watchdog |
| 7. Completion | FRICTION | - | Exit 4 (PARTIAL) correct. Agent produced code but no commit/PR |
| 8. PR Quality | N/A | - | No PR created |

## Findings

### Finding 1: Zero progress output during agent execution

| Field | Value |
|-------|-------|
| Category | F3: Missing Feedback |
| Severity | P2 |
| Phase | 6 (Monitor) |
| Known Issue | #310 (heartbeat progress PR not merged) |

**Observed:** After "dispatch start agent" log at 23:04:54, the next output was "dispatch transition" at 23:12:44 — 7min 50s of silence. No heartbeat, no progress, no indication the agent is alive.

**Expected:** Periodic progress updates (~30s intervals) during agent execution showing the agent is active and what it's doing.

**Impact:** Operator cannot distinguish between a working agent and a stalled one. No indication whether to wait or intervene.

**Recommended Fix:** Merge PR #310 (heartbeat progress for dispatch --wait).

---

### Finding 2: Agent produced work but didn't commit or create PR

| Field | Value |
|-------|-------|
| Category | F1: Silent Failure |
| Severity | P1 |
| Phase | 7 (Completion) |
| Known Issue | #349 (partial), #369 (NEW — root cause investigation) |

**Observed:** Agent modified 3 files (2 modified, 1 new) with a correct implementation of the ReconcileStatus function plus comprehensive tests. But the agent did not `git add`, `git commit`, `git push`, or create a PR. Exit code 4 (PARTIAL) correctly detected uncommitted changes.

**Expected:** Agent should complete the full workflow: implement → test → commit → push → PR.

**Impact:** Work is done but stranded on the sprite. Operator must either manually complete delivery or re-dispatch. The code quality was actually good — the failure is purely in the git workflow.

**Recommended Fix:** Investigate why Claude Code on the sprite didn't complete git operations. Possible causes: (1) git auth not configured on sprite, (2) token limit reached before git steps, (3) branch creation/push permissions missing. File new issue for investigation.

---

### Finding 3: Watchdog shows 'active' after dispatch completed

| Field | Value |
|-------|-------|
| Category | F4: Stale State |
| Severity | P1 |
| Phase | 7 (Completion) |
| Known Issue | #367 (NEW) |

**Observed:** After dispatch returned exit 4 (completed with uncommitted changes), running `bb watchdog --sprite fern` still showed `state=active`. The dispatch is over but watchdog reports active work.

**Expected:** Watchdog should reflect actual dispatch state. After dispatch returns, sprite should show idle or completed.

**Impact:** Operator checking fleet health after dispatch gets stale information. Could lead to wrong decisions about redispatch or fleet capacity.

**Recommended Fix:** Dispatch completion should clear/update STATUS.json on the sprite. Watchdog reads this file to determine state — if dispatch finishes but doesn't clean up, watchdog reports stale state.

---

### Finding 4: `bb status` contradicts watchdog during active dispatch

| Field | Value |
|-------|-------|
| Category | F2: Confusing Output |
| Severity | P2 |
| Phase | 6 (Monitor) |
| Known Issue | #368 (NEW) |

**Observed:** During active agent execution on fern:
- `bb status fern` showed `State: idle`
- `bb watchdog --sprite fern` showed `state=active`

Two different commands give contradictory answers about the same sprite.

**Expected:** All status commands should agree on sprite state, or clearly indicate which dimension they're reporting (API state vs dispatch state).

**Impact:** Operator checking on dispatch progress gets conflicting information depending on which command they use.

**Recommended Fix:** Either (1) `bb status` should also read STATUS.json to incorporate dispatch state, or (2) the output should clearly label which state source is being displayed.

---

### Finding 5: Composition sprites unreachable despite warm status

| Field | Value |
|-------|-------|
| Category | F9: Infrastructure Fragility |
| Severity | P1 |
| Phase | 5 (Dispatch) |
| Known Issue | #351, #360 |

**Observed:** thorn shows `idle/running` in fleet status but dispatch preflight correctly detected it's unreachable: "sprite thorn is not responding (signal: killed)". fern was reachable. bramble and moss show `unknown`.

**Expected:** Fleet status should verify actual connectivity, not just report Fly.io API status. Currently 1 of 4 composition sprites is reachable.

**Impact:** Operator must attempt dispatch to discover which sprites are actually available. The preflight fix (#362) correctly prevents wasted dispatch, but the discovery is trial-and-error.

**Recommended Fix:** File is known (#351, #360). Fleet status should include a connectivity probe in its assessment.

---

### Finding 6: thorn shows idle state + running status (contradiction)

| Field | Value |
|-------|-------|
| Category | F2: Confusing Output |
| Severity | P2 |
| Phase | 2 (Fleet Health) |
| Known Issue | #352 (the issue we dispatched to fix) |

**Observed:** `thorn 🟢 idle running - -` — state is idle but status is running. Other idle sprites show warm.

**Expected:** Idle sprites should show warm, not running. (This is exactly what issue #352 describes.)

**Impact:** "Running" implies active work, eroding operator trust in fleet status.

**Recommended Fix:** The agent actually implemented the correct fix (ReconcileStatus function) but it's stranded as uncommitted changes on fern. See Finding 2.

---

## Regression Check

| Issue | Description | Status |
|-------|-------------|--------|
| #277 | Stale TASK_COMPLETE | FIXED — clean_signals step in pipeline |
| #293 | --wait polling loops | FIXED — wait returned promptly |
| #294 | Oneshot exits with zero effect | IMPROVED — agent produced changes (exit 4 not exit 0) |
| #296 | Proxy health check no diagnostics | UNTESTED — no proxy failure encountered |
| #320 | Stdout pollution breaks JSON | UNTESTED — --json flag not used in this run |

## Timeline

| Time | Event |
|------|-------|
| 23:03:40 | Phase 1 — build (0.9s) |
| 23:03:42 | Phase 2 — fleet status (1.4s) |
| 23:04:00 | Phase 3 — issue selection |
| 23:04:17 | Phase 4 — credential validation via thorn dry-run |
| 23:04:17 | Phase 5a — dispatch to thorn (FAILED: unreachable) |
| 23:04:49 | Phase 5b — dispatch to fern (started) |
| 23:04:54 | Phase 6 — agent started on fern |
| 23:08:56 | Phase 6 — watchdog probe (active, 4min elapsed) |
| 23:12:44 | Phase 7 — agent completed, exit 4 (PARTIAL) |
| 23:12:58 | Phase 7 — post-dispatch watchdog (still shows active) |

## Raw Output

<details>
<summary>Dispatch to thorn (failed — unreachable)</summary>

```
time=2026-02-14T23:04:17.765-06:00 level=INFO msg="dispatch transition" sprite=thorn from=pending event=machine_ready to=ready
sprite "thorn" is not responding (executing on sprite "thorn": running sprite exec -s thorn -o misty-step -- bash -ceu echo ok: signal: killed ())
```

</details>

<details>
<summary>Dispatch to fern (completed partial)</summary>

```
time=2026-02-14T23:04:49.524-06:00 level=INFO msg="dispatch transition" sprite=fern from=pending event=machine_ready to=ready
time=2026-02-14T23:04:50.466-06:00 level=INFO msg="dispatch validate env" sprite=fern
time=2026-02-14T23:04:50.835-06:00 level=INFO msg="dispatch clean signals" sprite=fern
time=2026-02-14T23:04:51.256-06:00 level=INFO msg="dispatch setup repo" sprite=fern repo=https://github.com/misty-step/bitterblossom.git
time=2026-02-14T23:04:52.669-06:00 level=INFO msg="dispatch upload prompt" sprite=fern path=/home/sprite/workspace/.dispatch-prompt.md
time=2026-02-14T23:04:53.034-06:00 level=INFO msg="dispatch transition" sprite=fern from=ready event=prompt_uploaded to=prompt_uploaded
time=2026-02-14T23:04:53.441-06:00 level=INFO msg="dispatch ensure proxy" sprite=fern
time=2026-02-14T23:04:53.848-06:00 level=INFO msg="dispatch proxy ready" sprite=fern url=http://localhost:4000
time=2026-02-14T23:04:54.243-06:00 level=INFO msg="captured pre-exec HEAD SHA" sprite=fern sha=84472087c9365af3cb281bdc5e44b555551bfa75
time=2026-02-14T23:04:54.243-06:00 level=INFO msg="dispatch start agent" sprite=fern mode=oneshot
time=2026-02-14T23:12:44.785-06:00 level=INFO msg="dispatch transition" sprite=fern from=prompt_uploaded event=agent_started to=running
time=2026-02-14T23:12:44.785-06:00 level=INFO msg="dispatch transition" sprite=fern from=running event=oneshot_complete to=completed
time=2026-02-14T23:12:45.577-06:00 level=INFO msg="calculated work delta" sprite=fern commits=0 prs=0 has_changes=false dirty_files=3

=== Task Complete ===
Sprite: fern
State: completed
Status: PARTIAL (uncommitted changes in 3 file(s))
dispatch completed with uncommitted changes in 3 file(s)
```

Exit code: 4

</details>

<details>
<summary>Uncommitted changes on fern</summary>

```
 M cmd/bb/status.go
 M internal/lifecycle/status.go
?? internal/lifecycle/status_reconcile_test.go

 cmd/bb/status.go             |  2 +-
 internal/lifecycle/status.go | 35 +++++++++++++++++++++++++++++++++++
 2 files changed, 36 insertions(+), 1 deletion(-)
```

Agent implemented ReconcileStatus() function with comprehensive unit and integration tests.

</details>

## Recommendations

### Immediate (P0-P1)

- **Investigate agent git workflow failure** — the agent implemented the fix correctly but couldn't commit/push. Need to diagnose whether this is git auth, token limits, or workspace config. File new issue.
- **Fix stale watchdog state after dispatch** — dispatch completion should update STATUS.json. File new issue.
- **Merge PR #310** — heartbeat progress eliminates 8-minute silence gap.
- **Address sprite fleet degradation** — only 1 of 4 composition sprites reachable. Merge #308 (resilient transport) and investigate thorn/bramble/moss connectivity.

### Next Sprint (P2)

- **Unify status and watchdog state** — operators shouldn't get conflicting answers from different commands.
- **File #352 fix manually** — the agent's code is stranded on fern. Retrieve and complete the PR locally.

### Backlog (P3)

- None identified this run.
