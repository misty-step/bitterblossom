# Evaluation Criteria

Per-phase rubric for the e2e dispatch shakedown. Score each criterion as:

- **PASS** — works correctly, no friction
- **FRICTION** — works but with unnecessary difficulty, confusion, or delay
- **FAIL** — broken, wrong result, or blocks progress

## Phase 1: Build

| Criterion | PASS | FRICTION | FAIL |
|-----------|------|----------|------|
| Exit code | 0 | 0 with warnings | Non-zero |
| Duration | <30s | 30-120s | >120s or hang |
| Binary usable | Runs `--help` | Runs with deprecation warnings | Crashes or missing |

## Phase 2: Fleet Health

| Criterion | PASS | FRICTION | FAIL |
|-----------|------|----------|------|
| Status command | Returns structured output | Returns but format unclear | Errors or hangs |
| Sprite availability | 1+ sprites show `warm` | Sprites exist but status ambiguous | No sprites or all `unknown` |
| Response time | <10s | 10-30s | >30s or timeout |

## Phase 3: Issue Selection

| Criterion | PASS | FRICTION | FAIL |
|-----------|------|----------|------|
| Issue fetch | `gh issue view` returns full body | Body truncated or needs flags | Command fails |
| Context embedding | Issue body fits in prompt cleanly | Requires manual editing | Too large or unparseable |

## Phase 4: Credential Validation

| Criterion | PASS | FRICTION | FAIL |
|-----------|------|----------|------|
| Dry-run completes | Shows plan, no errors | Completes with warnings | Credential error blocks dispatch |
| Error messages | Clear, actionable | Present but vague | Missing or misleading |
| Token resolution | Automatic from env | Requires manual export | Fails silently |

## Phase 5: Dispatch

| Criterion | PASS | FRICTION | FAIL |
|-----------|------|----------|------|
| Confirmation output | Shows sprite, prompt summary, repo | Partial info | No confirmation or wrong info |
| Immediate errors | None | Non-blocking warnings | Blocking error |
| Skill mount | Skills mounted correctly | Mounted with warnings | Mount fails silently |

## Phase 6: Monitor

| Criterion | PASS | FRICTION | FAIL |
|-----------|------|----------|------|
| Progress messages | Appear every ~30s | Irregular or >60s gaps | No progress output |
| Stdout/stderr separation | Clean when `--json` used | Minor pollution | JSON broken by progress text |
| Stall detection | Watchdog catches stalls | Manual intervention needed | Stall undetectable |
| Status consistency | `bb status` matches reality | Minor lag | Status contradicts actual state |

## Phase 7: Completion

| Criterion | PASS | FRICTION | FAIL |
|-----------|------|----------|------|
| Exit code accuracy | Reflects actual outcome | Always 0 regardless | Wrong non-zero code |
| Signal file state | Correct signal present | Signal present but stale | Missing or contradictory |
| Duration | Within timeout | Close to timeout boundary | Exceeded timeout silently |
| Output summary | Clear success/failure message | Ambiguous completion | No completion message |

## Phase 8: PR Quality

| Criterion | PASS | FRICTION | FAIL |
|-----------|------|----------|------|
| PR created | Yes, correct repo and branch | Created but wrong target | Not created despite success signal |
| Commits | Meaningful, conventional format | Present but messy | Empty or squash-only |
| CI status | Passing | Pending | Failing |

## Overall Health Grade

| Grade | Criteria |
|-------|----------|
| **A** | 7+ phases PASS, 0 FAIL |
| **B** | 5-6 phases PASS, 0 FAIL |
| **C** | 3-4 phases PASS, or 1 FAIL |
| **D** | <3 phases PASS, or 2+ FAIL |
