# MEMORY

## Guardrails

- 2026-02-20: Never run keychain-enumeration commands (`security dump-keychain`, brute-force `security find-generic-password` loops). If auth is missing, fail fast and request explicit user action.

## Filed Issues

### 2026-02-18 e2e-test shakedown

- #410 `fix: logs --json must emit parseable JSON only` (NEW)
- #411 `fix: fleet status should surface busy sprites before dispatch` (NEW)
- #405 commented (expired token error still misleading)
- #407 commented (need true dispatch dry-run)
- #285 commented (missing TASK_COMPLETE regression signal)
- #293 commented (wait/poll hang regression signal)
- #320 commented (json-output pollution class)
- #388 commented (streaming regression signal)

### 2026-02-19 e2e-test shakedown

- #415 `fix: avoid false off-rails abort while waiting on CI checks` (NEW)
- #285 commented + reopened (missing TASK_COMPLETE regression signal)
- #293 commented + reopened (completion-loop regression signal)
- #298 commented + reopened (exit-code mismatch regression signal)
- #406 commented (timeout grace-period behavior reconfirmed)

### 2026-02-19 fresh-sprite e2e shakedown

- #418 `fix: bb setup should not require persona file matching fresh sprite name` (NEW)
- #419 `fix: dispatch outcome summary reports open_prs=0 despite newly created PR` (NEW)
- #420 `fix: dispatch marks TASK_COMPLETE success before PR CI status is known` (NEW)

### 2026-02-20 fresh-sprite e2e shakedown

- #419 commented (open_pr_count/pr_number still resolves to zero with open PR)
- #420 commented (green-check gate bypassed when PR detection fails)
- #422 `fix: bb should support sprite-cli auth fallback when Fly token exchange is unauthorized` (NEW)

### 2026-02-20 post-fix e2e reruns

- #419 commented (verified fixed: dispatch summaries now report open_prs/pr_number correctly)
- #420 commented (verified fixed: non-green PR checks now return non-zero)
- #422 commented (fallback via `fly auth token` works without manual refresh)
- #416 commented (off-rails still fires during PR-check wait; abort messaging vs termination mismatch)
- #293 commented (post-completion wait stall signal reproduced)
- #425 `fix: dispatch should avoid stale-branch git conflict spirals on sprite` (NEW)
