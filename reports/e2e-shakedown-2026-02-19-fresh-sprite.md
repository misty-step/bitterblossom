# E2E Shakedown Report

## Summary

| Field | Value |
|-------|-------|
| Date | 2026-02-19 |
| Sprite | `e2e-0219123628` (freshly created) |
| Issue | #405 - fix: improve expired token error message |
| Total Duration | ~3m57s (18:36:28-18:40:25 UTC) |
| Overall Grade | C |
| Findings Count | P0: 0 / P1: 2 / P2: 1 / P3: 0 |

## Phase Results

| Phase | Score | Duration | Notes |
|-------|-------|----------|-------|
| 1. Build | PASS | ~1s | `go build -o ./bin/bb ./cmd/bb` succeeded; binary usable |
| 2. Fleet Health | PASS | ~1s | `bb status` showed fresh sprite `warm` and reachable |
| 3. Issue Selection | PASS | ~12s | `gh issue view 405` returned full body; prompt embedded verbatim |
| 4. Credential Validation | PASS | ~17s | Smoke dispatch completed and wrote `TASK_COMPLETE` |
| 5. Dispatch | PASS | ~2m7s | Real issue dispatch produced commit and PR #417 |
| 6. Monitor | PASS | ~2m7s | Active streaming output; no observed >60s silence |
| 7. Completion | FRICTION | immediate after task | Exit code was 0 and `TASK_COMPLETE` found, but summary field contradicted PR reality (`open_prs=0`) |
| 8. PR Quality | FAIL | ~1m post-dispatch | PR #417 created correctly, but CI `Go Checks` failed |

## Findings

### Finding 1: Fresh sprite setup blocked by persona filename coupling

| Field | Value |
|-------|-------|
| Category | F7: Flag & CLI Ergonomics |
| Severity | P1 |
| Phase | Pre-phase / setup prerequisite |
| Known Issue | #418 (NEW) |

**Observed:** `bb setup e2e-0219123628 --repo misty-step/bitterblossom` failed with `persona not found: sprites/e2e-0219123628.md`.

**Expected:** Fresh sprite setup should work without creating a repo file matching the sprite name.

**Impact:** Hard block on fresh-sprite e2e flow until manual persona file workaround.

**Recommended Fix:** Add explicit persona selection (`--persona`) and/or default fallback persona resolution.

---

### Finding 2: Dispatch summary reports `open_prs=0` even when PR exists

| Field | Value |
|-------|-------|
| Category | F2: Confusing Output |
| Severity | P2 |
| Phase | 7 |
| Known Issue | #419 (NEW) |

**Observed:** Final output listed PR #417 in `work produced`, but `dispatch outcome` reported `open_prs=0`.

**Expected:** Summary counters should match discovered artifacts.

**Impact:** Operator and automation cannot trust completion summary fields.

**Recommended Fix:** Reconcile PR discovery and summary rendering; add regression test.

---

### Finding 3: Dispatch returns success before PR CI status is green

| Field | Value |
|-------|-------|
| Category | F1: Silent Failure |
| Severity | P1 |
| Phase | 7-8 |
| Known Issue | #420 (NEW) |

**Observed:** Dispatch exited `0` with `TASK_COMPLETE`, but PR #417 later showed failing `Go Checks`.

**Expected:** Completion semantics should clearly represent merge-readiness, or provide a strict mode that gates success on CI.

**Impact:** False-positive completion for autonomous issue execution.

**Recommended Fix:** Add CI-aware completion mode and explicit outcome state in final summary.

## Regression Check

Known issues from previous shakedowns. Mark each as REGRESSED, FIXED, or UNCHANGED.

| Issue | Description | Status |
|-------|-------------|--------|
| #277 | Stale TASK_COMPLETE | FIXED |
| #293 | --wait polling loops | FIXED |
| #294 | Oneshot exits with zero effect | FIXED |
| #296 | Proxy health check no diagnostics | UNCHANGED (not exercised in this run) |
| #320 | Stdout pollution breaks JSON | UNCHANGED (not exercised in this run) |

## Timeline

| Time (UTC) | Event |
|------------|-------|
| 18:36:28 | Created fresh sprite `e2e-0219123628` |
| 18:36:45 | `bb setup` failed: missing persona file for sprite name |
| 18:37:06 | Setup succeeded after temporary manual persona workaround |
| 18:37:45 | Fleet status confirmed sprite `warm` |
| 18:38:02 | Credential validation dispatch completed |
| 18:38:18 | Real dispatch started for issue #405 |
| 18:40:11 | PR #417 created by sprite |
| 18:40:25 | `TASK_COMPLETE` detected; dispatch exited 0 |
| 18:41:25 | PR CI `Go Checks` failed |

## Raw Output

<details>
<summary>Dispatch command and key output</summary>

```text
starting claude run (timeout 25m0s)...
dispatch timeout window: requested=25m0s grace=5m0s effective=30m0s
[ralph] harness=claude model=sonnet-4.6 mode=plugin
...
PR is open at https://github.com/misty-step/bitterblossom/pull/417.
...
dispatch outcome: task_complete=true blocked=false branch="fix/expired-token-hint" dirty_files=2 commits_ahead=1 open_prs=0
=== task completed: TASK_COMPLETE signal found ===
```

Full transcript: `/tmp/e2e3_phase5_dispatch_405.txt`

</details>

## Recommendations

### Immediate (P0-P1)

- Fix fresh-sprite setup persona resolution blocker (#418).
- Add CI-aware completion semantics so success can represent merge-ready output (#420).

### Next Sprint (P2)

- Fix dispatch summary PR counter mismatch (`open_prs`) (#419).

### Backlog (P3)

- Add explicit json-observability e2e check once auth path is stable for this branch.
