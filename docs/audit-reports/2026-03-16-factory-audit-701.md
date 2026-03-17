# Factory Audit Report

## Summary

- Date: 2026-03-16
- Run ID: run-701-1773693000
- Issue: #701 — [P1] Delete run-once and loop CLI commands
- PR: #705 — fix: remove deprecated conductor run commands
- Worker: bb-builder (Codex, gpt-5.4)
- Reviewers: Cerberus (pass), CodeRabbit (no comments), Gemini Code Assist (COMMENTED)
- Terminal State: **Merged** (v1.70.1)

## Timeline

| Time (UTC) | Event | Notes |
|---|---|---|
| 20:28:14 | Conductor start | 3 sprites loaded |
| 20:28:15 | bb-fixer healthy | — |
| 20:28:15 | bb-polisher needs setup | Reconciler `bb setup` fails — CWD mismatch |
| 20:28:16 | bb-builder needs setup | Same failure |
| — | Manual sprite setup | Ran `bb setup` from repo root |
| 20:29:49 | Conductor restart | All 3 sprites healthy |
| 20:30:00 | **Lease acquired #701** | Also leased #702 simultaneously |
| 20:30:01 | Workspace prepared | 1s |
| 20:30:01 | Builder dispatched | Codex gpt-5.4, --yolo |
| 20:35:50 | PR #705 opened | CI runs started |
| 20:40:47 | All CI green | Cerberus PASS, merge-gate PASS |
| 20:41:02 | Polisher dispatches to #705 | Before conductor knew builder was done |
| 20:41:39 | Builder complete (conductor) | 11m 38s builder time |
| 20:48:35 | Polisher comments on #705 | Addressed automated feedback, ran tests |
| 20:48:52 | **Merged** by orchestrator | Governance passed |
| 20:48:59 | Retro complete | 1 finding, 0 actions |
| 20:50:13 | Release v1.70.1 | Automated release |

Total: **19 minutes** lease-to-merge.

## Secondary Run: #702

Run-702 (issue #702: delete dead scripts) launched concurrently on the same builder sprite. Opened PR #706 but the Codex agent exited non-zero at 20:55:03. Run marked failed. PR #706 left orphaned.

## Findings

### Finding 1: Reconciler relative path crash (fixed in-session)

- Severity: P1
- Status: **Fixed** — `Path.expand` added to `find_bb/0`
- Observed: `System.cmd("../bin/bb", ...)` fails with `:enoent` even though `File.exists?("../bin/bb")` returns true
- Root cause: Erlang/OTP does not resolve `..` in executable paths for `System.cmd/3`
- Evidence: Conductor crashed on first boot attempt

### Finding 2: Reconciler CWD mismatch (#708)

- Severity: P2
- Issue: #708
- Observed: `bb setup` fails because `base/settings.json` not found
- Expected: Reconciler should run `bb setup` from repo root, not conductor/
- Why it matters: Prevents automatic sprite provisioning on boot

### Finding 3: Double-dispatch to same builder (#709)

- Severity: P2
- Issue: #709
- Observed: Runs #701 and #702 both dispatched to bb-builder concurrently
- Expected: One active run per builder, or documented concurrency
- Why it matters: Resource contention; run-702 failed (possibly related)

### Finding 4: Polisher spin loop (#710)

- Severity: P2
- Issue: #710
- Observed: PR #697 polished every 60s, completing instantly each time
- Expected: Polisher tracks "already polished" and skips
- Why it matters: Wastes sprite resources, floods logs

### Finding 5: `bb logs` blind to Codex agents

- Severity: P2
- Status: Goes away with #703 (delete Go CLI)
- Observed: `bb logs bb-builder` reports "No active task" while Codex agents are running
- Why it matters: Operator has no observability into builder progress

### Finding 6: API keys in process list (#711)

- Severity: P2
- Issue: #711
- Observed: OPENAI_API_KEY visible in `ps aux` on sprites
- Why it matters: Secrets exposure on shared infrastructure

### Finding 7: Log noise (#712)

- Severity: P2
- Issue: #712
- Observed: ~72 lines/min of "worker busy" deferrals + git hints
- Why it matters: Drowns out actionable events (merge, phase transitions)

## Backlog Actions

- New issues: #708, #709, #710, #711, #712
- Fixed in-session: `find_bb/0` Path.expand (not yet committed)
- Existing issues referenced: #703 (Go CLI deletion subsumes #708 and Finding 5)

## Reflection

- What Bitterblossom did well:
  - Full pipeline executed: build → polish → merge → retro → release in 19 minutes
  - Governance worked correctly — orchestrator merged, not the polisher
  - Issue shaping ran in parallel, readying 6 issues for future runs
  - Retro ran automatically and identified a valid performance concern

- What felt brittle:
  - Boot requires manual sprite setup (CWD bug)
  - Two-language coupling (Go bb + Elixir conductor) causes path resolution bugs
  - No progress visibility during builds — operator can only wait
  - Polisher spinning wastes resources and obscures useful signal

- What should be simpler next time:
  - One-command boot with no manual sprite provisioning
  - One builder per run enforcement
  - Polisher idempotency tracking
  - Structured event log that separates signal from noise
