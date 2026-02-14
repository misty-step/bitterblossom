# E2E Shakedown Report

## Summary

| Field | Value |
|-------|-------|
| Date | 2026-02-14 |
| Sprite | fern (primary), thorn (retry) |
| Issue | #329 — fix: GitHub API Issue.Closed field doesn't exist in REST response |
| Total Duration | ~9 minutes (17:29:50 – 17:38:12) |
| Overall Grade | D |
| Findings Count | P0: 1 / P1: 3 / P2: 2 / P3: 0 |

## Phase Results

| Phase | Score | Duration | Notes |
|-------|-------|----------|-------|
| 1. Build | PASS | 0.58s | Clean, no warnings |
| 2. Fleet Health | FRICTION | 2.2s | Status doesn't verify connectivity; shows warm sprites that are unreachable |
| 3. Issue Selection | PASS | <5s | Clean fetch, context embeds without modification |
| 4. Credential Validation | PASS | ~1s | Dry-run shows execution plan, credentials resolve |
| 5. Dispatch | FAIL | ~2m (fern), ~3m (thorn) | Fern: I/O timeout during work delta. Thorn: proxy health check failed |
| 6. Monitor | FAIL | N/A | Sprite unreachable during and after dispatch |
| 7. Completion | FAIL | N/A | Exit code 3 on fern masks I/O error as "no new work"; Exit 1 on thorn |
| 8. PR Quality | N/A | - | No PR created |

## Findings

### Finding 1: Work delta I/O timeout misreported as "no new work"

| Field | Value |
|-------|-------|
| Category | F1: Silent Failure |
| Severity | P0 |
| Phase | 7 (Completion) |
| Known Issue | NEW (extension of #349) |

**Observed:** Fern dispatch exited with code 3 ("COMPLETE (no new work)") but the work delta check itself failed with `i/o timeout` when trying to capture post-exec HEAD SHA. The log shows `level=WARN msg="failed to calculate work delta"` but the exit code and status message don't distinguish "verified zero work" from "couldn't check."

**Expected:** When work delta verification fails, exit code should indicate verification failure (not "zero work"). Message should say "work delta check failed: could not verify outcome" rather than "no new work."

**Impact:** Operator sees "no new work" and assumes nothing happened. In reality, the agent may have committed work that we simply couldn't verify. False negative masquerading as a clean zero-work detection.

**Recommended Fix:** In the work delta code path, when `capturePostExecHEAD` fails, return a distinct error and exit code (e.g., exit 4 = "verification failed") rather than falling through to the zero-work path.

---

### Finding 2: Fleet status shows "warm" for unreachable sprites

| Field | Value |
|-------|-------|
| Category | F4: Stale State |
| Severity | P1 |
| Phase | 2 (Fleet Health) |
| Known Issue | #351 |

**Observed:** Fleet overview reports fern and thorn as "warm" / "idle". Both are unreachable — `bb status fern` hangs indefinitely, dispatch operations take 45s per step (TCP timeout pattern), and post-dispatch I/O times out.

**Expected:** Fleet status should verify actual connectivity, or at minimum label sprites with a "reachability unknown" indicator when last-probe is stale.

**Impact:** Operator selects a "warm" sprite for dispatch, wastes 3-5 minutes discovering it's unreachable only after dispatch fails.

---

### Finding 3: Proxy diagnostics return empty data on health check failure

| Field | Value |
|-------|-------|
| Category | F1: Silent Failure |
| Severity | P1 |
| Phase | 5 (Dispatch) |
| Known Issue | #350 (regression of #296) |

**Observed:** Thorn dispatch failed at proxy health check. Diagnostics section shows:
```
Memory:

Processes:

Proxy log (last 50 lines):

```
All fields empty. Zero diagnostic information despite explicit "Diagnostics" heading.

**Expected:** Diagnostics should return memory usage, process list, and proxy log. If the sprite is unreachable, the diagnostics section should explicitly say "sprite unreachable — diagnostics unavailable" rather than printing empty fields.

**Impact:** When proxy fails, operator has no information to debug. The empty diagnostics section suggests data was collected but found nothing, when in reality the collection itself failed silently.

---

### Finding 4: Every remote operation takes ~45s (TCP timeout cascade)

| Field | Value |
|-------|-------|
| Category | F9: Infrastructure Fragility |
| Severity | P1 |
| Phase | 5 (Dispatch) |
| Known Issue | Related to #351, #297 |

**Observed:** On thorn dispatch, each pipeline step took ~45 seconds:
- validate_env → clean_signals: 45s
- clean_signals → setup_repo: 46s
- setup_repo → upload_prompt: 45s
- upload_prompt → prompt_uploaded: 45s
- prompt_uploaded → ensure_proxy: 46s

This is consistent with a 30-second TCP timeout (Fly.io idle timeout per #144) followed by a 15-second retry. Each step appears to fail once and succeed on retry, or fail twice and accumulate timeout.

**Expected:** Transport should fail-fast when sprite connectivity is degraded, rather than burning 45s × N_steps. A preflight connectivity probe before entering the dispatch pipeline would catch this in <30s total rather than 3+ minutes.

**Recommended Fix:** Add a fast connectivity check (TCP connect or ICMP equivalent) as the first dispatch step. If the sprite doesn't respond within 5s, fail immediately with "sprite unreachable" rather than attempting the full pipeline.

---

### Finding 5: `bb status <sprite>` hangs indefinitely for unreachable sprite

| Field | Value |
|-------|-------|
| Category | F6: Timing & Performance |
| Severity | P2 |
| Phase | 6 (Monitor) |
| Known Issue | Related to #351 |

**Observed:** `bb status fern --format text` hung for >45 seconds with no output after "fetching detail for fern". Had to `timeout 15` or manually kill. Same for `bb status thorn`.

**Expected:** Status command should have a reasonable default timeout (10-15s) and report "sprite unreachable" rather than hanging.

**Impact:** Operator trying to diagnose a dispatch failure gets stuck in a second hang.

---

### Finding 6: thorn shows "idle" state + "running" status simultaneously

| Field | Value |
|-------|-------|
| Category | F2: Confusing Output |
| Severity | P2 |
| Phase | 2 (Fleet Health) |
| Known Issue | #352 |

**Observed:** Fleet overview shows `thorn: 🟢 idle / running`. These are contradictory — a sprite is either idle or running a task.

**Expected:** State and status should be consistent. If the VM is running but has no task, both should indicate "idle."

**Impact:** Operator can't tell whether thorn is available for dispatch.

---

## Regression Check

| Issue | Description | Status |
|-------|-------------|--------|
| #277 | Stale TASK_COMPLETE | FIXED — clean_signals step observed in logs |
| #293 | --wait polling loops | FIXED — fern dispatch completed within timeout |
| #296 | Proxy health check no diagnostics | REGRESSED — #350 confirms empty diagnostics still present |
| #320 | Stdout pollution breaks JSON | UNCHANGED — not tested (didn't use --json mode) |

## Timeline

| Time | Event |
|------|-------|
| 17:29:50 | Fern dispatch start — validate env |
| 17:29:55 | Clean signals |
| 17:29:56 | Setup repo |
| 17:29:59 | Upload prompt |
| 17:30:00 | Ensure proxy — proxy ready at localhost:4000 |
| 17:30:01 | Capture pre-exec HEAD SHA (0aa44ba) |
| 17:30:01 | Start oneshot agent |
| 17:30:46 | Agent oneshot_complete → state=completed (45s execution) |
| 17:31:51 | Work delta FAILED: I/O timeout capturing post-exec HEAD |
| 17:31:51 | Exit code 3: "COMPLETE (no new work)" |
| 17:34:25 | Thorn dispatch start — validate env |
| 17:35:10 | Clean signals (45s) |
| 17:35:56 | Setup repo (46s) |
| 17:36:41 | Upload prompt (45s) |
| 17:37:26 | Prompt uploaded (45s) |
| 17:38:12 | Ensure proxy — FAILED: context deadline exceeded (46s) |
| 17:38:12 | Exit code 1 |

## Raw Output

<details>
<summary>Fern dispatch output</summary>

```
time=2026-02-14T17:29:50.733-06:00 level=INFO msg="dispatch transition" sprite=fern from=pending event=machine_ready to=ready
time=2026-02-14T17:29:50.733-06:00 level=INFO msg="dispatch validate env" sprite=fern
time=2026-02-14T17:29:54.915-06:00 level=INFO msg="dispatch clean signals" sprite=fern
time=2026-02-14T17:29:55.582-06:00 level=INFO msg="dispatch setup repo" sprite=fern repo=https://github.com/misty-step/bitterblossom.git
time=2026-02-14T17:29:59.078-06:00 level=INFO msg="dispatch upload prompt" sprite=fern path=/home/sprite/workspace/.dispatch-prompt.md
time=2026-02-14T17:29:59.600-06:00 level=INFO msg="dispatch transition" sprite=fern from=ready event=prompt_uploaded to=prompt_uploaded
time=2026-02-14T17:30:00.108-06:00 level=INFO msg="dispatch ensure proxy" sprite=fern
time=2026-02-14T17:30:00.641-06:00 level=INFO msg="dispatch proxy ready" sprite=fern url=http://localhost:4000
time=2026-02-14T17:30:01.140-06:00 level=INFO msg="captured pre-exec HEAD SHA" sprite=fern sha=0aa44ba787e5c4fa7ae557c0c2b79d7353951bae
time=2026-02-14T17:30:01.140-06:00 level=INFO msg="dispatch start agent" sprite=fern mode=oneshot
time=2026-02-14T17:30:46.629-06:00 level=INFO msg="dispatch transition" sprite=fern from=prompt_uploaded event=agent_started to=running
time=2026-02-14T17:30:46.629-06:00 level=INFO msg="dispatch transition" sprite=fern from=running event=oneshot_complete to=completed
time=2026-02-14T17:31:51.737-06:00 level=WARN msg="failed to calculate work delta" sprite=fern error="capture post-exec HEAD SHA: sprite: command failure: executing on sprite \"fern\": running sprite exec -s fern -o misty-step -- bash -ceu cd '/home/sprite/workspace/bitterblossom' && git rev-parse HEAD 2>/dev/null || echo '': exit status 1 (Error: failed to start sprite command: failed to connect: read tcp 192.168.1.108:52420->169.155.48.226:443: i/o timeout)"

=== Task Complete ===
Sprite: fern
State: completed
Task: misty-step/bitterblossom: Fix GitHub issue #329...
Status: COMPLETE (no new work)
dispatch completed but produced no new work
```

</details>

<details>
<summary>Thorn dispatch output</summary>

```
time=2026-02-14T17:34:25.687-06:00 level=INFO msg="dispatch transition" sprite=thorn from=pending event=machine_ready to=ready
time=2026-02-14T17:34:25.688-06:00 level=INFO msg="dispatch validate env" sprite=thorn
time=2026-02-14T17:35:10.972-06:00 level=INFO msg="dispatch clean signals" sprite=thorn
time=2026-02-14T17:35:56.271-06:00 level=INFO msg="dispatch setup repo" sprite=thorn repo=https://github.com/misty-step/bitterblossom.git
time=2026-02-14T17:36:41.540-06:00 level=INFO msg="dispatch upload prompt" sprite=thorn path=/home/sprite/workspace/.dispatch-prompt.md
time=2026-02-14T17:37:26.817-06:00 level=INFO msg="dispatch transition" sprite=thorn from=ready event=prompt_uploaded to=prompt_uploaded
time=2026-02-14T17:38:12.196-06:00 level=INFO msg="dispatch ensure proxy" sprite=thorn
dispatch: ensure proxy: proxy health check failed: proxy health check failed: context deadline exceeded

=== Diagnostics ===
Memory:

Processes:

Proxy log (last 50 lines):

=== Next steps ===
• Check sprite status: bb status thorn
• View full proxy log: sprite exec thorn -- tail -f /home/sprite/.bb/proxy.log
• Check system logs: sprite exec thorn -- journalctl -u proxy 2>/dev/null || dmesg | tail
• Restart sprite if OOM suspected: bb stop thorn && bb start thorn
```

</details>

## Recommendations

### Immediate (P0-P1)

- **P0 — Fix work delta error path**: When `capturePostExecHEAD` fails, return a distinct exit code (e.g., 4) and message ("verification failed") instead of falling through to exit 3 ("no new work"). The current behavior is a silent failure that masks possible agent work.
- **P1 — Preflight connectivity probe**: Add a 5-second TCP connectivity check before entering the dispatch pipeline. Fail fast with "sprite unreachable" instead of burning 45s × N steps.
- **P1 — Fix empty diagnostics (#350)**: When diagnostic data collection fails (sprite unreachable), explicitly report "diagnostics unavailable — sprite unreachable" instead of printing empty fields.
- **P1 — Add timeout to `bb status <sprite>`**: Default 15-second timeout, report "unreachable" on expiry.

### Next Sprint (P2)

- **P2 — Fleet status connectivity verification (#351)**: Fleet overview should probe actual reachability, not just report Fly.io machine state.
- **P2 — Consistent state/status in fleet output (#352)**: Resolve the "idle" + "running" contradiction for thorn.

### Backlog (P3)

- None identified this run.
