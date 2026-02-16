# E2E Shakedown Report

## Summary

| Field | Value |
|-------|-------|
| Date | 2026-02-16 |
| Branch | rewrite/sdk-v2 |
| Sprite | fern |
| Issue | #290 — docs: deprecate or update base/prompts/ralph-loop-v2.md |
| Total Duration | ~22 min (09:35 → 10:10) |
| Overall Grade | D |
| Findings Count | P0: 2 / P1: 4 / P2: 2 / P3: 0 |

## Phase Results

| Phase | Score | Duration | Notes |
|-------|-------|----------|-------|
| 1. Build | PASS | 0.4s | Clean exit, binary works |
| 2. Fleet Health | FRICTION | 11s | Stale FLY_API_TOKEN, missing SPRITES_ORG, no --format flag |
| 3. Issue Selection | PASS | 10s | Issue #290 fetched cleanly |
| 4. Credential Validation | FRICTION | ~1m | CombinedOutput deadlock required code fix; --timeout ineffective |
| 5. Dispatch | FRICTION | ~10s | Dispatch started but no streaming output from agent |
| 6. Monitor | FAIL | 20m | Zero output, can't monitor sprite independently |
| 7. Completion | FAIL | N/A | Agent produced zero work, no signals, no PR |
| 8. PR Quality | N/A | N/A | No PR created |

## Findings

### Finding 1: sprites-go CombinedOutput() deadlocks

| Field | Value |
|-------|-------|
| Category | F1: Silent Failure |
| Severity | P0 |
| Phase | 4 (Credential Validation) |
| Known Issue | NEW → #387 |

**Observed:** `dispatch.go:101` calls `syncCmd.CombinedOutput()` which deadlocks. Both goroutines in the sprites-go websocket layer (wsCmd.Wait and wsCmd.readMessage) block in selects waiting for channels that never produce. The process panics with "all goroutines are asleep — deadlock!"

**Expected:** CombinedOutput should return the command output or an error within the context timeout.

**Impact:** Dispatch is completely broken on the rewrite branch. No dispatch can succeed without this fix.

**Recommended Fix:** Replace `CombinedOutput()` with `Output()` at dispatch.go:101. The sync script already merges stderr via `2>&1`. Filed workaround applied during this shakedown. Upstream sprites-go bug should be reported separately.

---

### Finding 2: Dispatch streaming produces zero agent output

| Field | Value |
|-------|-------|
| Category | F3: Missing Feedback |
| Severity | P0 |
| Phase | 6 (Monitor) |
| Known Issue | NEW → #388 |

**Observed:** After ralph starts iteration 1 (`[ralph] iteration 1 / 50`), zero bytes of output are received for 20+ minutes. The `claude -p` process output does not stream through the sprites-go websocket to the local terminal. The ralph process eventually terminates with no indication of what happened.

**Expected:** Agent output should stream in real-time through the websocket, showing what the agent is doing.

**Impact:** Complete blindness during dispatch. Operator has no visibility into agent progress, errors, or completion. This makes the dispatch pipeline unusable for debugging or monitoring.

**Recommended Fix:** Investigate sprites-go SDK streaming behavior. Possibly related to websocket buffering, keepalive, or binary message framing. May need to force line-buffering on the remote side (e.g., `stdbuf -oL` or `script` wrapper).

---

### Finding 3: Agent produces zero effect (no commits, no signals)

| Field | Value |
|-------|-------|
| Category | F1: Silent Failure |
| Severity | P1 |
| Phase | 7 (Completion) |
| Known Issue | #294, #369 |

**Observed:** After dispatch completes, the bitterblossom workspace on fern shows: no new branch, no new commits, no signals (TASK_COMPLETE, BLOCKED.md), no PR. The only change is the uploaded `.dispatch-prompt.md`.

**Expected:** Agent should create a branch, make commits, open a PR, and write TASK_COMPLETE on success (or BLOCKED.md if stuck).

**Impact:** Dispatch consumes time and resources with no output. Without streaming output (Finding 2), the failure is completely invisible.

**Recommended Fix:** Cannot diagnose root cause without streaming output. Fix Finding 2 first, then re-run to see why the agent fails. Likely related to Claude Code configuration on the sprite (missing settings.json, auth, or permissions).

---

### Finding 4: --timeout doesn't enforce within iterations

| Field | Value |
|-------|-------|
| Category | F6: Timing & Performance |
| Severity | P1 |
| Phase | 4 (Credential Validation) |
| Known Issue | NEW → #389 |

**Observed:** `--timeout 1m` dispatched with MAX_TIME_SEC=60, but ITER_TIMEOUT_SEC defaults to 900s (15min). The time check in ralph.sh only runs between iterations (line 34), not during them. The first iteration can run for up to 15 minutes regardless of the total timeout.

**Expected:** `--timeout 1m` should ensure the entire dispatch completes within ~1 minute.

**Impact:** Short timeouts for testing are meaningless. Operators can't quickly test dispatch without committing to a 15+ minute minimum wait.

**Recommended Fix:** Set `ITER_TIMEOUT_SEC=min(ITER_TIMEOUT_SEC, MAX_TIME_SEC)` in the ralph env, or wrap the entire ralph loop in `timeout` at the dispatch level.

---

### Finding 5: Cannot monitor sprite while dispatch is running

| Field | Value |
|-------|-------|
| Category | F9: Infrastructure Fragility |
| Severity | P1 |
| Phase | 6 (Monitor) |
| Known Issue | NEW → #390 |

**Observed:** Running `bb status fern` while a dispatch is active returns "sprite unreachable" or "use of closed network connection". The sprites-go SDK's connection pool is exhausted or corrupted by the long-running dispatch websocket.

**Expected:** Operator should be able to independently check sprite status during dispatch.

**Impact:** Combined with Finding 2 (no streaming output), the operator is completely blind during dispatch. No way to see what's happening on the sprite.

**Recommended Fix:** Use separate connection pools for status checks vs long-running commands, or allow multiple concurrent websocket connections per sprite.

---

### Finding 6: .env.bb has stale token and missing SPRITES_ORG

| Field | Value |
|-------|-------|
| Category | F5: Credential Pain |
| Severity | P1 |
| Phase | 2 (Fleet Health) |
| Known Issue | NEW → #391 |

**Observed:** `.env.bb` generated by `scripts/onboard.sh` contains a stale `FLY_API_TOKEN` that fails token exchange. Also sets `FLY_ORG=misty-step` but not `SPRITES_ORG` which the rewrite code requires. Must use `fly auth token` for a fresh token and manually export `SPRITES_ORG`.

**Expected:** `source .env.bb` should provide all credentials needed for dispatch.

**Impact:** First-run experience is broken. Operator must debug token exchange failures and discover the missing env var.

**Recommended Fix:** Either (a) regenerate .env.bb with fresh tokens and add SPRITES_ORG, or (b) have `spriteToken()` fall back to `FLY_ORG` when `SPRITES_ORG` is not set, or (c) add `fly auth token` as a dynamic fallback.

---

### Finding 7: Status command picks wrong workspace on multi-workspace sprites

| Field | Value |
|-------|-------|
| Category | F2: Confusing Output |
| Severity | P2 |
| Phase | 6 (Monitor) |
| Known Issue | NEW → #392 |

**Observed:** `bb status fern` shows workspace `bb322/` (from a previous dispatch) instead of `bitterblossom/` (the current dispatch target). The status command uses `ls -d | head -1` which returns the first directory alphabetically.

**Expected:** Status should show the most recently active workspace, or the one relevant to the current/last dispatch.

**Impact:** Status output is misleading. Operator sees stale workspace data while the actual dispatch workspace shows different state.

**Recommended Fix:** Accept `--repo` flag on status to select workspace, or check modification times to find the most recently active workspace.

---

### Finding 8: No --format flag on rewrite branch

| Field | Value |
|-------|-------|
| Category | F7: Flag & CLI Ergonomics |
| Severity | P2 |
| Phase | 2 (Fleet Health) |
| Known Issue | #321 (related) |

**Observed:** `bb status --format text` fails with "unknown flag: --format". The rewrite branch has no `--format` flag on any command. The e2e-test skill template references `--format text` which doesn't exist.

**Expected:** All commands should support `--format=json|text` per issue #321.

**Impact:** Skill templates and automation are broken. Parseable output (JSON) not available.

**Recommended Fix:** Add `--format` flag to status and dispatch commands per #321 spec.

---

## Regression Check

| Issue | Description | Status |
|-------|-------------|--------|
| #277 | Stale TASK_COMPLETE | FIXED (dispatch.go line 106 cleans signals) |
| #293 | --wait polling loops | N/A (rewrite uses synchronous streaming, no polling) |
| #294 | Oneshot exits with zero effect | REGRESSED (agent produced zero work) |
| #296 | Proxy health check no diagnostics | N/A (rewrite doesn't use proxy checks) |
| #320 | Stdout pollution breaks JSON | N/A (no JSON output mode on rewrite) |

## Timeline

| Time | Event |
|------|-------|
| 09:35:26 | Phase 1 start — build |
| 09:35:27 | Phase 1 PASS — 0.4s build |
| 09:35:36 | Phase 2 start — fleet health |
| 09:35:36 | --format flag fails |
| 09:36:12 | Stale token fails |
| 09:36:47 | Fresh token works — fleet status returned |
| 09:37:04 | fern detail status OK |
| 09:37:16 | Phase 3 — issue list fetched |
| 09:37:30 | Phase 3 — issue #290 body fetched |
| 09:38:30 | Phase 4 — first dispatch attempt |
| 09:38:35 | CombinedOutput DEADLOCK — fatal |
| 09:39:00 | Fix applied (Output instead of CombinedOutput) |
| 09:40:15 | Phase 4 retry — 1m timeout dispatch |
| 09:43:15 | Ralph iteration 1 started |
| 09:46:30 | Killed — 1m timeout not honored |
| 09:48:05 | Phase 5 — real dispatch (20m timeout, issue #290) |
| 09:48:15 | Ralph iteration 1 started on fern |
| 09:49:00 | Phase 6 — monitoring begins, zero output |
| 09:52:00 | Status check — fern unreachable |
| 10:05:00 | Still zero output — connection appears dead |
| 10:09:00 | Dispatch killed, fern reachable again |
| 10:09:30 | Post-mortem: zero work produced, no signals, no PR |
| 10:10:00 | Phase 7 — FAIL, shakedown complete |

## Raw Output

<details>
<summary>Dispatch command and output</summary>

```
09:48:05
exchanging fly token for sprites token (org=misty-step)...
probing fern...
syncing repo misty-step/bitterblossom...
starting ralph loop (max 50 iterations, 20m0s timeout, harness=claude)...
[ralph] iteration 1 / 50 at 2026-02-16T15:48:15+00:00
(no further output received — killed after 20+ minutes)
```

</details>

<details>
<summary>CombinedOutput deadlock trace</summary>

```
fatal error: all goroutines are asleep - deadlock!

goroutine 1 [select]:
github.com/superfly/sprites-go.(*wsCmd).Wait(0x140001ae480)
  sprites-go@v0.0.0-20260206213632-8176adff485b/websocket.go:363 +0x60
github.com/superfly/sprites-go.(*Cmd).Wait(0x1400026e2c0)
  sprites-go@v0.0.0-20260206213632-8176adff485b/exec.go:353 +0x10c
github.com/superfly/sprites-go.(*Cmd).Run(0x1400026e2c0)
  sprites-go@v0.0.0-20260206213632-8176adff485b/exec.go:172 +0x38
github.com/superfly/sprites-go.(*Cmd).CombinedOutput(0x1400026e2c0)
  sprites-go@v0.0.0-20260206213632-8176adff485b/exec.go:433 +0x98
main.runDispatch(...)
  dispatch.go:101 +0x7f4
```

</details>

## Recommendations

### Immediate (P0-P1)

- Fix CombinedOutput deadlock (Finding 1) — already worked around with Output()
- Fix streaming output (Finding 2) — investigate sprites-go SDK websocket buffering
- Diagnose zero-effect agent failure (Finding 3) — requires Finding 2 to be fixed first
- Fix --timeout enforcement (Finding 4) — cap ITER_TIMEOUT_SEC at MAX_TIME_SEC
- Fix status during dispatch (Finding 5) — separate connection pools
- Fix .env.bb credential flow (Finding 6) — SPRITES_ORG fallback or token refresh

### Next Sprint (P2)

- Fix multi-workspace status (Finding 7) — accept --repo flag or use mtime
- Add --format flag (Finding 8) — per #321 spec
