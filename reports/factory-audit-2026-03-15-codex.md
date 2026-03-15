# Factory Audit Report — First Codex Dispatch

## Summary

- Date: 2026-03-15
- Run ID: run-646-1773588869 (retry after run-646-1773588248)
- Issue: #646 (Retro: persist analysis results in Store)
- PR: #669 (OPEN — not merged)
- Worker: bb-builder
- Harness: Conductor.Codex (gpt-5.4, --yolo, medium reasoning)
- Fixer: bb-fixer (never triggered)
- Polisher: bb-polisher (never triggered)
- Terminal State: **INCOMPLETE** — pr_opened only; governance/polish/merge never ran

## Audit Verdict: PARTIAL FAILURE

The factory successfully dispatched Codex and got a PR with green CI. But the pipeline stopped at `pr_opened`. Five of nine expected steps never executed. This is not a Codex-specific problem — it's a `run-once` limitation: RunServer treats `pr_opened` as terminal by design, and governance/polish/merge only run in `loop` mode.

## Expected Pipeline vs Actual

| Step | Expected | Actual | Verdict |
|------|----------|--------|---------|
| Assign issue to bot | Yes | **No** | FAILED |
| Mark issue In Progress | Yes | **No** | FAILED |
| Builder dispatch | PR opened | PR #669 opened | PASSED |
| CI green | Yes | Yes | PASSED |
| Review comments addressed | Yes | Partial (CodeRabbit only) | PARTIAL |
| `/simplify` on PR diff | Yes | **Never ran** | FAILED |
| `/pr-polish` before LGTM | Yes | **Never ran** | FAILED |
| `lgtm` label applied | Yes | **Never applied** | FAILED |
| Squash merge | Yes | **Never happened** | FAILED |

## Timeline

| Time (UTC) | Event | Notes |
|------------|-------|-------|
| 15:24:01 | First run start | run-646-1773588248 |
| 15:24:18 | **Builder failed: 401 Unauthorized** | CODEX_API_KEY vs OPENAI_API_KEY |
| 15:33:00 | Auth fix merged | PR #668 |
| 15:34:29 | **Retry run start** | run-646-1773588869 |
| 15:34:30 | Lease acquired, builder dispatched | |
| ~15:39:00 | PR #669 opened by Codex | CI green on first push |
| 15:45:01 | Builder complete: `pr_opened` | Conductor exits — no further phases |
| 15:45:01 | **Run ends** | Issue not assigned, no polish, no merge |

## Findings

### Finding 1: CODEX_API_KEY auth mismatch (P0, fixed)

- Severity: P0
- Status: Fixed in PR #668
- Observed: 401 Unauthorized — Codex reads `CODEX_API_KEY`, not `OPENAI_API_KEY`
- Root cause: Research error — web sources conflated login-time and runtime env var names

### Finding 2: `run-once` only runs the builder phase (P1)

- Severity: P1
- Status: Known design — but the factory audit assumed full pipeline
- Observed: Conductor exited at `pr_opened` without governance, polish, or merge
- Expected: Full pipeline through merge
- Why it matters: `run-once` is the natural audit command, but it only proves builder dispatch. To audit the full pipeline, you must use `loop` mode or manually drive fixer/polisher/orchestrator.
- Evidence: RunServer moduledoc: "The builder opens a PR and exits. Governance (CI, reviews, merge) is handled by the orchestrator's label-driven merge loop."

### Finding 3: Issue not assigned or marked In Progress (P2)

- Severity: P2
- Status: Not implemented in conductor
- Observed: Issue #646 remained unassigned with no project status change
- Expected: Conductor claims the issue before dispatch (prevents double-dispatch)
- Why it matters: Without assignment, another run could pick up the same issue

### Finding 4: No `/simplify` or `/pr-polish` ran (P1)

- Severity: P1
- Status: These are polisher-phase responsibilities
- Observed: PR #669 has no simplification pass or polish
- Expected: Polisher sprite runs with `reasoning_effort: "high"` and applies both
- Why it matters: The PR may have quality issues that polish would catch
- Root cause: Polisher GenServer only runs in `loop` mode

### Finding 5: No LGTM label → no merge (P1)

- Severity: P1
- Status: Blocked by Finding 4
- Observed: PR #669 has no `lgtm` label, orchestrator merge loop never triggered
- Expected: Polisher adds `lgtm` after successful polish → orchestrator merges
- Root cause: Same as Finding 4 — polisher never ran

### Finding 6: Codex builder output was clean (Positive)

- Severity: Positive
- Observed: 2 files, 143 additions, CI green first push, review comments addressed
- Why it matters: Validates Codex as a viable builder harness

### Finding 7: Codex builder time ~10.5 min (P3, informational)

- Severity: P3
- Observed: 10.5 min for a small issue (vs ~5-8 min Claude Code typical)
- Note: First run with cold prompt cache

### Finding 8: No persona files for bb-* sprites (P3)

- Severity: P3
- Observed: `warning: no persona for "bb-builder", using fallback sprites/beaker.md`

### Finding 9: Flaky StoreTest in CI (P3, pre-existing)

- Severity: P3
- Observed: `test get_run returns error for missing run` failed once, passed on rerun

## Backlog Actions

- **PR #668 merged**: CODEX_API_KEY auth fix (Finding 1)
- **Existing issue comment needed**: The `run-once` vs full-pipeline gap (Finding 2) should be documented or addressed — either `run-once` should optionally drive governance, or the factory audit skill should use `loop` mode
- **No new issues filed yet** — Findings 3-5 are governance gaps that may already be tracked

## Reflection

- **What Bitterblossom did well**: Harness abstraction held perfectly. Codex dispatched through the same pipeline as Claude Code with zero orchestration changes. Builder output was clean.
- **What felt brittle**: The operator (me) assumed `run-once` would drive the full pipeline. It doesn't. The audit should have either used `loop` mode or manually invoked the polisher and merge steps after the builder completed. The factory audit skill should make this explicit.
- **What should be simpler next time**: (1) Add a `mix conductor smoke --worker <sprite>` for pre-dispatch auth validation. (2) The factory audit skill should either use `loop` mode or explicitly drive all five conductor authorities after builder completes. (3) Document that `run-once` = builder only, `loop` = full pipeline.
