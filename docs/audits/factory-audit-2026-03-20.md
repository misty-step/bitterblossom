# Factory Audit Report

## Summary

- Date: 2026-03-20
- Run ID: run-709-1774020328 (primary observed), run-710-1774020329
- Issue: #709 (dispatched by conductor), #710 (concurrent dispatch)
- PR: none (both runs failed)
- Worker: bb-builder (recovered mid-audit), bb-fixer (unreachable), bb-polisher (unreachable)
- Reviewers: n/a (fixer/polisher unreachable)
- Terminal State: **both runs failed — OpenAI API key returns 401 Unauthorized**

## Context

This audit began as investigation of a ~24-hour undetected factory outage (all sprites unreachable). Root cause: Fly.io DFW region had elevated machine start failure rates (incident 2026-03-20 07:26-12:45 UTC). After the sprite CLI was upgraded (rc37 → rc41) and DFW capacity recovered, bb-builder came back online. The conductor was started and dispatched two runs, both of which failed due to an expired OpenAI API key.

## Timeline

| Time (UTC) | Event | Notes |
|------|-------|-------|
| 2026-03-19 14:31 | prior runs picked | Runs #707 and #708 dispatched; conductor dies shortly after |
| 2026-03-19 14:39 | last heartbeat | Factory goes dark — no alerting |
| 2026-03-20 07:26 | Fly DFW incident begins | "Machines start failure rate is elevated in DFW" |
| 2026-03-20 12:45 | Fly DFW capacity added | "Provisioned new capacity in DFW" |
| 2026-03-20 14:30 | audit preflight | All 3 sprites 502; stale runs discovered |
| 2026-03-20 14:47 | sprite CLI upgraded | rc37 → rc41; bb-builder briefly wakes, then goes cold again |
| 2026-03-20 14:52 | stale runs expired | Manually expired run-707 and run-708 |
| 2026-03-20 15:24 | conductor started | bb-builder healthy; fixer/polisher still unreachable |
| 2026-03-20 15:25:28 | run-709 leased | Issue #709 picked (P2, not highest priority) |
| 2026-03-20 15:25:29 | run-710 leased | Issue #710 picked concurrently (P2) |
| 2026-03-20 15:25:34 | both dispatched | Two codex processes on bb-builder simultaneously |
| 2026-03-20 15:32:11 | both failed | 401 Unauthorized from api.openai.com; classified harness_unsupported |

## Findings

### Finding 1: No external conductor watchdog — 24h silent outage

- Severity: P1
- New issue: #746
- Observed: Factory dead for ~24 hours with no alert. Root cause: conductor process died.
- Expected: External watchdog detects conductor death within 10 minutes.
- Implementation: Register sprite URLs + conductor `/health` as canary-obs health check targets.
- Evidence: Last heartbeat 2026-03-19T14:38:59Z. Stale run threshold (65 min) never enforced.

### Finding 2: Sprites unreachable due to Fly DFW outage — no auto-recovery

- Severity: P1
- New issue: #747
- Observed: All 3 sprites return HTTP 502 from sprites.app due to Fly.io DFW capacity constraints.
- Root cause: Fly.io incident — "Machines failing to start in DFW" (07:26-12:45 UTC).
- Recovery required: sprite CLI upgrade (rc37 → rc41) + waiting for DFW capacity restoration.
- Evidence: `websocket: bad handshake (HTTP 502)` for all sprites; Fly status page confirmed.

### Finding 3: OpenAI API key expired — all builder dispatches fail

- Severity: P0
- New issue: #750
- Observed: Codex gets `401 Unauthorized: Missing bearer or basic authentication in header` from api.openai.com. Both runs fail as `harness_unsupported`.
- Expected: `check-env` validates API key liveness, not just presence.
- Why it matters: Every dispatch burns 7 minutes before failing. No productive work possible.
- Evidence: Manual codex test on sprite reproduced the 401 with 5 retries.

### Finding 4: Concurrent dispatch to same builder sprite

- Severity: P2
- Existing issue: commented on #709
- Observed: Two codex processes (PIDs 1203, 1204) ran simultaneously on bb-builder. The conductor dispatched run-709 and run-710 within 0.3 seconds of each other.
- Expected: One builder per sprite. #745 ("single-pr builder lanes") did not prevent this.
- Evidence: `pgrep -la codex` on bb-builder showed two concurrent processes.

### Finding 5: Priority labels ignored in issue selection

- Severity: P2
- Existing issue: #733
- Observed: Conductor picked #709 and #710 (both P2) while P1 issues #741, #742 were available.
- Expected: P1 issues dispatched before P2.
- Evidence: `gh issue list` shows #741 (P1) and #742 (P1) open and unassigned.

### Finding 6: Stale runs persist after conductor death

- Severity: P2
- Existing issue: commented on #714
- Observed: Runs for closed issues #707 and #708 remained as `building/pending` for 24+ hours.
- Fix applied during audit: manually expired via `Store.expire_stale_run/4`.
- Note: Conductor DID expire these on restart (they show `status: failed` after boot), confirming the startup reconciliation works — but only when the conductor actually restarts.

## Backlog Actions

- New issues: #746 (conductor watchdog, P1), #747 (sprite auto-recovery, P1), #750 (API key expired, P0)
- Existing issues commented: #709 (concurrent dispatch evidence), #714 (stale run evidence)
- Priority changes: #714 recommended P2 → P1

## Reflection

- What Bitterblossom did well: Startup stale-run reconciliation worked correctly when the conductor restarted. Event logging captured precise timestamps. The retro analysis auto-commented on #709 after the dispatch failure. Workspace cleanup ran after failures.
- What felt brittle: (1) Multiple simultaneous failures (Fly DFW + expired API key + stale CLI) made diagnosis hard — each layer masked the next. (2) No preflight validation of API key liveness. (3) The 7-minute failure loop (codex retries 5x with backoff) wastes time when the key is simply invalid.
- What should be simpler next time: (1) canary-obs health checks on sprites + conductor would catch outages in minutes. (2) `check-env` should make a lightweight API call to validate the key, not just check `System.get_env`. (3) The sprite CLI version should be pinned or auto-upgraded in the reconciler.
