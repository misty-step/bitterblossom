# E2E Shakedown Report

## Summary

| Field | Value |
|-------|-------|
| Date | 2026-02-18 |
| Sprite | bramble (primary), fern/clover attempted |
| Issue | #406 - fix: timeout grace period is fixed 5min regardless of --timeout value |
| Total Duration | 22m29s (18:24:02Z -> 18:46:31Z) |
| Overall Grade | D |
| Findings Count | P0: 0 / P1: 3 / P2: 3 / P3: 1 |

## Phase Results

| Phase | Score | Duration | Notes |
|-------|-------|----------|-------|
| 1. Build | PASS | 2s | `go build -o ./bin/bb ./cmd/bb` exit 0. `./bin/bb --help` exit 0. |
| 2. Fleet Health | FAIL | ~31s | `bb status --format text` unknown flag. `bb status` failed with misleading unauthorized error until fresh `FLY_API_TOKEN` generated. |
| 3. Issue Selection | PASS | ~2m | `gh issue view 406` returned full body. Embedded verbatim into prompt. |
| 4. Credential Validation | FAIL | 55s | No dry-run support. `dispatch ... "test"` hit active-loop blocks and one run had to be interrupted. |
| 5. Dispatch | FAIL | 5m43s (primary) | `dispatch bramble` entered iteration 1, then no meaningful progress/completion; manual Ctrl-C required. |
| 6. Monitor | FAIL | 5m43s | >60s silence gaps, repeated `BashTool pre-flight` warnings, off-rails countdown did not converge to terminal exit. |
| 7. Completion | FAIL | N/A | No terminal exit from primary dispatch without manual interrupt. Signals absent despite pushed branch/commit. |
| 8. PR Quality | FAIL | N/A | Branch + commit pushed (`4187523`) but no PR created. |

## Findings

### Finding 1: Expired token error remains misleading

| Field | Value |
|-------|-------|
| Category | F5: Credential Pain |
| Severity | P1 |
| Phase | 2 |
| Known Issue | #405 (commented) |

**Observed:** `bb status` failed with `token exchange failed: unauthorized (set SPRITES_ORG if not 'personal')`.

**Expected:** Error should indicate likely expired `FLY_API_TOKEN` and remediation.

**Impact:** Operators waste time debugging org config instead of rotating token.

**Recommended Fix:** Keep #405 open; prioritize explicit token-expiry hint.

---

### Finding 2: Fleet status hides busy sprites

| Field | Value |
|-------|-------|
| Category | F3: Missing Feedback |
| Severity | P2 |
| Phase | 2 / 4 |
| Known Issue | #411 (NEW) |

**Observed:** Fleet status showed `clover` warm, but dispatch immediately failed with `active dispatch loop detected`.

**Expected:** Status should expose dispatch availability (`busy` vs `idle`) pre-dispatch.

**Impact:** Sprite selection becomes trial-and-error and causes failed dispatch attempts.

**Recommended Fix:** Add busy signal into fleet status output (#411).

---

### Finding 3: Credential validation still starts real work

| Field | Value |
|-------|-------|
| Category | F7: Flag & CLI Ergonomics |
| Severity | P2 |
| Phase | 4 |
| Known Issue | #407 (commented) |

**Observed:** `bb dispatch <sprite> "test"` performs real dispatch behavior; no safe validation-only path.

**Expected:** Dry-run should validate tokens/connectivity/busy-check without starting agent.

**Impact:** Validation can create junk work, stalls, and manual cleanup.

**Recommended Fix:** Implement `--dry-run` as tracked in #407.

---

### Finding 4: Dispatch stream stalls and does not self-terminate reliably

| Field | Value |
|-------|-------|
| Category | F6: Timing & Performance |
| Severity | P1 |
| Phase | 5 / 6 |
| Known Issue | #293 and #388 (commented) |

**Observed:** After iteration 1, output mostly stopped; repeated `BashTool` pre-flight warnings appeared; command did not reach terminal state before manual Ctrl-C.

**Expected:** Off-rails/no-output handling should deterministically exit with clear status.

**Impact:** Operator cannot trust wait behavior; automation blocks indefinitely.

**Recommended Fix:** Re-open/patch #293/#388 path; enforce bounded termination once off-rails triggers.

---

### Finding 5: Work can land without completion protocol convergence

| Field | Value |
|-------|-------|
| Category | F1: Silent Failure |
| Severity | P1 |
| Phase | 7 |
| Known Issue | #285 (commented) |

**Observed:** On `bramble`, branch `fix/grace-period-proportional-to-timeout` + commit `4187523` were pushed, but `TASK_COMPLETE*` and `BLOCKED.md` were absent and local dispatch never completed.

**Expected:** Completion protocol should emit terminal signal and CLI should converge to exit.

**Impact:** Successful work appears hung/unknown; operator must inspect remote git manually.

**Recommended Fix:** Reinforce signal contract and add fallback completion detection when branch/commit/PR evidence exists.

---

### Finding 6: `logs --json` output still not machine-parseable

| Field | Value |
|-------|-------|
| Category | F2: Confusing Output |
| Severity | P2 |
| Phase | 6 |
| Known Issue | #410 (NEW), related #320 (commented) |

**Observed:** `bb logs bramble --json` prints `exchanging fly token ...` plain text on stdout.

**Expected:** JSON mode should emit only JSON on stdout; progress text on stderr.

**Impact:** Breaks tooling pipelines that parse logs output.

**Recommended Fix:** Route token-exchange/progress text to stderr in JSON mode (#410).

---

### Finding 7: Skill/runbook command contract drift

| Field | Value |
|-------|-------|
| Category | F8: Documentation Gap |
| Severity | P3 |
| Phase | 2 / 5 / 6 |
| Known Issue | NEW (not filed, P3) |

**Observed:** Runbook references flags/commands absent in current CLI (`status --format`, `dispatch --execute --wait`, `watchdog`).

**Expected:** Skill docs and CLI surface should match.

**Impact:** Extra operator friction and false-start failures during shakedown.

**Recommended Fix:** Update skill docs + examples to current command surface.

## Regression Check

Known issues from previous shakedowns. Mark each as REGRESSED, FIXED, or UNCHANGED.

| Issue | Description | Status |
|-------|-------------|--------|
| #277 | Stale TASK_COMPLETE | UNCHANGED (stale observed on fern pre-run; no false-positive completion observed) |
| #293 | --wait polling loops | REGRESSED |
| #294 | Oneshot exits with zero effect | FIXED (work effect observed) |
| #296 | Proxy health check no diagnostics | UNCHANGED (not directly exercised this run) |
| #320 | Stdout pollution breaks JSON | REGRESSED (logs path) |

## Timeline

| Time | Event |
|------|-------|
| 18:24:02 | Phase 1 start - build |
| 18:24:04 | Phase 1 complete (PASS) |
| 18:24:13 | Phase 2 start - fleet health (`--format` failure) |
| 18:24:18 | Token exchange unauthorized with stale env token |
| 18:24:34 | Fresh token generated; fleet status returns warm sprites |
| 18:27:13 | Phase 4 validation attempt on fern |
| 18:27:15 | Fern rejected: active dispatch loop detected |
| 18:31:37 | Dispatch attempt on clover |
| 18:31:39 | Clover rejected: active dispatch loop detected |
| 18:34:40 | Dispatch attempt on fern started |
| 18:35:45 | Off-rails no-output warning |
| 18:40:03 | Fern run manually interrupted |
| 18:40:48 | Dispatch attempt on bramble started |
| 18:40:56 | Ralph iteration 1 |
| 18:41:56 | Off-rails no-output warning |
| 18:43:26 | First BashTool pre-flight warning |
| 18:46:31 | Bramble run manually interrupted |
| 18:47:xx | Status shows pushed branch/commit on bramble without completion signal |

## Raw Output

<details>
<summary>Dispatch command and output (primary bramble run)</summary>

```bash
./bin/bb dispatch bramble "<issue-406 prompt>" --repo misty-step/bitterblossom --timeout 25m
```

```text
exchanging fly token for sprites token (org=misty-step)...
probing bramble...
syncing repo misty-step/bitterblossom...
starting ralph loop (max 50 iterations, 25m0s timeout, harness=claude)...
[ralph] harness=claude model=default
[ralph] harness=claude model=default[ralph] iteration 1 / 50 at 2026-02-18T18:40:56+00:00
[off-rails] no output for 1m0s (abort in 4m0s)
⚠️  [BashTool] Pre-flight check is taking longer than expected. Run with ANTHROPIC_LOG=debug to check for failed or slow API requests.
[off-rails] no output for 53s (abort in 4m7s)
⚠️  [BashTool] Pre-flight check is taking longer than expected. Run with ANTHROPIC_LOG=debug to check for failed or slow API requests.
⚠️  [BashTool] Pre-flight check is taking longer than expected. Run with ANTHROPIC_LOG=debug to check for failed or slow API requests.
⚠️  [BashTool] Pre-flight check is taking longer than expected. Run with ANTHROPIC_LOG=debug to check for failed or slow API requests.
⚠️  [BashTool] Pre-flight check is taking longer than expected. Run with ANTHROPIC_LOG=debug to check for failed or slow API requests.
```

</details>

## Recommendations

### Immediate (P0-P1)

- Fix completion convergence after off-rails/no-output paths (#293 regression + #285 regression evidence).
- Improve credential failure diagnostics for expired Fly token (#405).

### Next Sprint (P2)

- Make `logs --json` stdout cleanly machine-parseable (#410).
- Surface busy/active-loop state in fleet status (#411).
- Add dispatch dry-run mode for safe credential validation (#407).

### Backlog (P3)

- Sync skill/runbook examples to current CLI surface.
