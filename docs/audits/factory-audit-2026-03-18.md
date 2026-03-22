# Factory Audit Report

## Summary

- Date: 2026-03-18
- Run IDs: `run-687-1773867375`, `run-702-1773867376`
- Issues: #687 (rename sprites), #702 (delete dead scripts)
- PRs: #727 (merged), #728 (orphaned), #729 (orphaned)
- Workers: bb-builder, bb-fixer (idle), bb-polisher
- Reviewers: Cerberus (4 reviewers), CodeRabbit, Gemini
- Terminal State: #687 shipped, #702 stalled with duplicate PRs

## Timeline

| Time (UTC-4) | Event | Notes |
|------|-------|-------|
| 15:55:54 | Conductor boot | 3/3 sprites healthy |
| 15:55:55 | Polling started | No label filter — all 15+ open issues eligible |
| 15:56:15 | Lease #687 on bb-builder | |
| 15:56:16 | Lease #702 on bb-builder | Concurrent dispatch to same sprite (#709) |
| 15:56:16 | Both builders dispatched | Two codex processes on one sprite |
| 16:02:30 | PR #727 created (issue #687) | Builder opened PR mid-execution |
| 16:06:55 | PR #728 created (issue #702) | On factory/ branch |
| 16:08:27 | Run #687 builder complete | 12 min, zero progress events (#714) |
| 16:09:29 | Deferral noise flood | 16 lines per cycle for 8 deferred issues (#712) |
| 16:09:29 | Logger error | Disk space exhaustion on operator machine |
| ~16:13 | Cerberus verdict #727: WARN | 3/4 pass, Testing warned |
| ~16:14 | Polisher dispatched | bb-polisher starts codex (reasoning_effort=high) |
| ~16:16 | Cerberus verdict #728: FAIL→PASS | Testing+Correctness failed, then Testing skipped on timeout |
| 16:18:35 | Polisher pushes fix on #727 | Addressed Testing warning |
| 16:19:34 | PR #729 created (issue #702) | **Duplicate PR** on cx/ branch, not tracked by conductor |
| ~16:25 | LGTM added to #727 | Polisher approved |
| 16:29:44 | PR #727 merged | Conductor squash-merged. Issue #687 closed |
| ~16:30 | Polisher moves to #729 | Works on **untracked** PR instead of #728 |
| 16:34:24 | Polisher pushes fix on #729 | Addressed Gemini review |
| ~16:40 | Polisher finishes | No LGTM on either #728 or #729 |
| ~16:45 | Conductor stopped | Manual stop for audit |

## Findings

### Finding: Builder creates duplicate PRs

- Severity: P1
- New issue: #730
- Observed: Builder codex for #702 created PR #728 (factory/ branch) and PR #729 (cx/ branch)
- Expected: One PR per run, on the conductor-assigned factory/ branch only
- Why it matters: The conductor cannot govern work it doesn't track. Creates orphaned PRs and splits polisher effort.
- Evidence: PR #728 and #729 both reference issue #702, created 13 min apart during same builder run

### Finding: Polisher not robust to duplicate PRs

- Severity: P2
- New issue: #731
- Observed: With two PRs open for #702, polisher picked one arbitrarily and didn't converge on either
- Expected: Polisher detects duplicate PRs, picks best candidate, logs the situation
- Why it matters: Polisher effort wasted. Root cause is #730 (duplicate PRs), but polisher should be resilient.
- Evidence: Polisher ran ~10 min across #729 without adding LGTM, #728 untouched

### Finding: Fixer never dispatched

- Severity: P2
- New issue: #732
- Observed: PR #728 had Cerberus FAIL verdict. Fixer sprite idle throughout entire audit (0 processes at every check)
- Expected: Fixer dispatched to address Cerberus failures
- Why it matters: CI failures go unaddressed. Polisher can't compensate — it's meant for polish, not correctness fixes.
- Evidence: bb-fixer had 0 codex processes at every sprite check during 50-min audit

### Finding: Issue selection ignores priority and assignees

- Severity: P2
- New issue: #733
- Observed: Conductor selected #687 (P2) and #702 (P1, assigned to phaedrus) while P2 bugs (#709-#716) were available
- Expected: P1 bugs over P2 features; skip human-assigned issues
- Why it matters: Factory burns compute on low-priority work while critical bugs wait
- Evidence: Issue selection log shows #687 and #702 picked first out of 15+ eligible issues

### Finding: Concurrent dispatch to same sprite (confirmed)

- Severity: P2
- Existing issue: #709 (commented)
- Observed: Both runs dispatched to bb-builder within 1 second
- Evidence: 6 codex processes on bb-builder simultaneously

### Finding: Deferral log noise (confirmed)

- Severity: P2
- Existing issue: #712 (commented)
- Observed: 16 log lines per polling cycle (2 per deferred issue x 8 issues)
- Evidence: Conductor output at 16:09:29

### Finding: No builder progress monitoring (confirmed)

- Severity: P2
- Existing issue: #714 (commented)
- Observed: 12-minute blind gap during builder execution
- Evidence: Zero events between dispatch and completion in conductor log

## Backlog Actions

- New issues filed: #730, #731, #732, #733
- Existing issues commented: #709, #712, #714
- Priority changes: None proposed (existing priorities appropriate)
- Cleanup needed: Close PR #728 and #729 (orphaned, issue #702 still open)

## Reflection

- What Bitterblossom did well:
  - Issue #687 shipped end-to-end in 33 minutes (dispatch → merge)
  - Polisher correctly identified Cerberus Testing warning and pushed a fix
  - LGTM label added only after review feedback addressed
  - Squash merge executed cleanly, issue closed automatically
  - All three sprites booted healthy in <2 seconds

- What felt brittle:
  - Codex creating branches outside the conductor's control (cx/ vs factory/)
  - Polisher scanning all PRs instead of conductor-tracked ones
  - No fixer dispatch despite clear CI failures
  - Issue selection is effectively random within the eligible set
  - Disk space exhaustion silently killed conductor logging

- What should be simpler next time:
  - Single-PR-per-run invariant enforced mechanically, not by convention
  - Fixer should be automatic when Cerberus fails, not a separate manual concern
  - Priority-based issue selection should be the default, not an opt-in feature
  - The polisher's PR scope should be the conductor's tracked set, period
