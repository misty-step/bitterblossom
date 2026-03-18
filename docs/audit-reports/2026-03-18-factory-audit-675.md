# Factory Audit Report #3

## Summary

- Date: 2026-03-18
- Target Issue: #702 (queued — #675/#676 selected first by priority)
- Runs Observed: #675 (3 attempts, 3rd succeeded to pr_opened), #676 (3 attempts, all blocked), #680 (started)
- PRs: #723 (issue #675, polished, CI green, NOT merged), #722 (issue #676, polished, CI green, NOT merged)
- Terminal State: **No merges.** Polisher never adds `lgtm` label so merge loop cannot trigger.

## Timeline

| Time (UTC) | Event | Notes |
|---|---|---|
| 13:25:02 | Conductor start | 3 sprites healthy |
| 13:25:07 | Stale run-711 lease expired | From audit #2 |
| 13:25:29 | run-675 attempt 1 starts | PR adopted |
| 13:25:30 | run-676 attempt 1 starts | Both to bb-builder |
| 13:34:01 | run-675 builder complete | **Blocked**: external reviews pending |
| 13:38:15 | Fixer dispatches to PR #723 | CI went red |
| 13:41:03 | Fixer completes PR #723 | 2m 48s |
| 13:50:45 | run-676 attempt 1 complete | Builder timeout |
| 13:51:20 | run-675 attempt 2 starts | — |
| 13:51:21 | run-676 attempt 2 starts | — |
| 13:52:26 | Polisher dispatches to PR #723 | CI green |
| 13:58:20 | run-676 attempt 2 blocked | Cerberus Correctness still running |
| 14:01:38 | run-675 attempt 2 blocked | Cerberus still pending |
| 14:02:34 | run-675 attempt 3 starts | — |
| 14:02:35 | run-676 attempt 3 starts | — |
| 14:02:36 | Polisher completes PR #723 | — |
| 14:03:27 | Polisher dispatches to PR #722 | — |
| 14:11:27 | **run-675 attempt 3 builder reports ready** | PR #723 |
| 14:18:28 | Polisher completes PR #722 | — |
| 14:19:28 | Polisher dispatches to PR #723 (again) | — |
| ~14:25 | Conductor stopped | No merges |

Total runtime: ~60 minutes. 6 builder dispatches, 0 merges.

## Findings

### Finding 1: Polisher never adds lgtm label (#724)

- Severity: **P1**
- Issue: #724
- Observed: PR #723 polished twice, all CI green, Cerberus PASS, zero labels. Merge loop requires `lgtm` label but polisher never adds it.
- Why it matters: This is the root cause of zero merges across audits #2 and #3.

### Finding 2: External review restart loop (strengthens #681)

- Severity: P1
- Existing issue: #681 (commented)
- Observed: Each builder retry pushes new commits that restart external reviews (Cerberus, CodeRabbit). Governance checks immediately, reviews still running, blocks.
- Delta from audit #2: #713 fixed repo CI timing. External review timing is the remaining blocker.

### Finding 3: Fleet status misreports bb-fixer as unreachable

- Severity: P2
- Observed: `bb status` fleet overview shows bb-fixer as `REACH: no`, but `bb status bb-fixer` succeeds with full details. The fleet probe gives a false negative.
- Why it matters: Operator confusion during preflight.

### Finding 4: Progress — run-675 reached pr_opened on 3rd attempt

- Severity: Positive
- Observed: The artifact protocol fix from #713 + retries eventually worked. Run-675 reached `pr_opened` status with PR #723.
- Why it matters: The builder pipeline IS functional. The merge gate is the only remaining blocker.

## Backlog Actions

- New issues: #724 (polisher lgtm label — P1)
- Commented: #681 (external review timing)

## Reflection

- What Bitterblossom did well:
  - All 3 sprites healthy on boot (first time in 3 audits without manual setup)
  - Fixer autonomously fixed CI on PR #723 (2m 48s)
  - Polisher completed work on both PRs
  - Builder reached pr_opened on 3rd attempt
  - Retro loop active — commented on #675 and #681 automatically
  - Stale lease from audit #2 was properly expired

- What failed:
  - Zero merges. The merge loop cannot trigger because the polisher never adds the `lgtm` label.
  - External review restart loop wastes 3 build cycles per issue.
  - #702 (our target) never got a slot — #675/#676 consumed all attempts.

- Critical path to first reliable merge:
  1. **#724** — polisher must add `lgtm` label (or merge loop must use alternative signal)
  2. **#681** — governance must poll/wait for external reviews instead of blocking immediately
  3. With both fixed, audit #1's 19-minute merge time should be reproducible

- Delta from audit #2:
  - Audit #2: 0 merges, governance never saw a successful builder artifact
  - Audit #3: 0 merges, but builder succeeded + polisher completed + CI green. Only the label gate remains.
  - The pipeline is 90% functional. The last 10% is the merge trigger.
