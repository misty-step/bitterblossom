# Quality Gateway — Autonomous Software Factory Safeguards

*The faster we move, the stronger the guardrails need to be.*

## Philosophy

High velocity × low quality = expensive rework. High velocity × high quality = compounding advantage.

An autonomous software factory shipping code 24/7 requires **more** safeguards than a human team, not fewer. Every layer of automated quality control we add is a layer of trust we earn. The goal: make it structurally difficult to ship bugs.

## The Quality Stack

### Layer 1: Pre-Commit (Developer/Sprite)
**Goal: Catch problems before they leave the branch.**

- [ ] **Strong typing everywhere** — TypeScript strict mode, Swift strict concurrency
- [ ] **Exhaustive linting** — ESLint (strictest config), SwiftLint, `tsc --noEmit`
- [ ] **Pre-commit hooks** — lint + type-check + format on every commit
- [ ] **Tests pass locally** — sprites must run tests before pushing
- [ ] **Branch protection** — no direct pushes to master/main, period

### Layer 2: CI/CD Pipeline (GitHub Actions)
**Goal: Automated, exhaustive verification on every PR.**

- [ ] **Build verification** — clean build with zero warnings (warnings-as-errors)
- [ ] **Test suite** — all unit + integration tests pass
- [ ] **Coverage enforcement** — 80% minimum, 90% target, block merge if below threshold
- [ ] **Linting** — full lint pass (not just changed files)
- [ ] **Type checking** — full type check pass
- [ ] **Security scanning** — dependency audit, secret detection
- [ ] **Cerberus review council** — 5 AI reviewers from different perspectives
  - Security reviewer (threat modeling, injection, auth)
  - Architecture reviewer (design, coupling, patterns)
  - Performance reviewer (complexity, memory, latency)
  - Quality reviewer (readability, maintainability, edge cases)
  - Test reviewer (coverage, assertions, scenarios)
- [ ] **All checks must pass** — zero tolerance for failures

### Layer 3: Review & Merge (Claw)
**Goal: Human-grade final review with taste and judgment.**

- [ ] **Codex 5.3 xhigh deep review** — architecture, correctness, style
- [ ] **Cross-PR impact analysis** — does this conflict with other open PRs?
- [ ] **Manual QA when possible** — build and test the actual app
- [ ] **Merge only when:** CI green + Cerberus approves + Codex approves
- [ ] **Squash merge** — clean commit history on master

### Layer 4: Post-Deploy Monitoring
**Goal: Catch problems that escape pre-deploy checks.**

- [ ] **Error tracking** — Sentry (or equivalent) on all deployed apps
- [ ] **Error rate alerting** — >5% error rate spike → automatic rollback + notification
- [ ] **Health checks** — HTTP health endpoints, uptime monitoring
- [ ] **Log aggregation** — structured logging, easily searchable
- [ ] **Performance monitoring** — latency p50/p95/p99 tracking
- [ ] **Webhooks for anomalies** — real-time alerts, not just polling
  - Sentry webhook → notify Claw of new error spikes
  - Fly.io alerts → notify on machine crashes/restarts
  - GitHub webhook → notify on failed deploys

### Layer 5: Continuous Quality Improvement
**Goal: The system gets better over time.**

- [ ] **Post-mortem on every bug** — what escaped and why? Update checks.
- [ ] **Coverage ratchet** — coverage threshold can only go up, never down
- [ ] **Lint rule additions** — when a new class of bug appears, add a lint rule
- [ ] **Test pattern library** — document testing patterns for sprites to follow
- [ ] **Cerberus prompt tuning** — improve reviewer prompts based on miss analysis

## Implementation Roadmap

### Phase 1: Foundation (Now)
- [x] CI/CD exists (GitHub Actions on vox, heartbeat)
- [x] Cerberus in progress (Phaedrus setting up)
- [x] Branch protection on repos
- [ ] **Ralph Loop v2** — sprites self-heal PRs (CI + reviews)
- [ ] **Claw final review** — Codex 5.3 reviews before merge
- [ ] **Coverage reporting** — add coverage to CI output

### Phase 2: Enforcement (Next Week)
- [ ] **Coverage thresholds** — enforce 80% minimum on all repos
- [ ] **Warnings-as-errors** — zero-warning policy
- [ ] **SwiftLint for Vox** — strict config
- [ ] **ESLint strict for TS projects** — all rules cranked up
- [ ] **Sentry integration** — error tracking on deployed apps
- [ ] **PR shepherd cron** — check open sprite PRs, re-dispatch if needed

### Phase 3: Observability (Week After)
- [ ] **Structured logging standard** — JSON logs with correlation IDs
- [ ] **Fly.io log drains** — aggregate logs to searchable store
- [ ] **Error rate dashboards** — in Overmind command center
- [ ] **Webhook alerting** — Sentry → Telegram alerts to Claw
- [ ] **Auto-rollback** — deploy fails health check → rollback + alert

### Phase 4: Automation (Ongoing)
- [ ] **Auto-revert on error spike** — >5% error rate → revert last deploy
- [ ] **Flaky test detection** — quarantine flaky tests, fix them
- [ ] **Dependency update bot** — automated dependency PRs (Renovate/Dependabot)
- [ ] **Performance regression detection** — benchmark on every PR
- [ ] **Coverage ratchet enforcement** — CI blocks if coverage drops

## Per-Project Requirements

### TypeScript Projects (Conviction, Overmind, etc.)
```json
// tsconfig.json
{
  "compilerOptions": {
    "strict": true,
    "noUncheckedIndexedAccess": true,
    "noImplicitOverride": true,
    "exactOptionalPropertyTypes": true,
    "noFallthroughCasesInSwitch": true,
    "forceConsistentCasingInFileNames": true
  }
}
```

### Swift Projects (Vox)
```yaml
# .swiftlint.yml
strict: true
opt_in_rules:
  - force_unwrapping
  - implicitly_unwrapped_optional
  - discouraged_optional_boolean
  - fatal_error_message
  - unowned_variable_capture
```

### All Projects
- README with build/test/deploy instructions
- CLAUDE.md with architecture and conventions
- .github/workflows/ci.yml with full check suite
- Pre-commit hooks (lint + type-check)
- Minimum 80% test coverage

## Metrics to Track

| Metric | Target | Alert Threshold |
|--------|--------|----------------|
| Test coverage | >90% | <80% blocks merge |
| CI pass rate | >95% | <90% investigate |
| Error rate (production) | <1% | >5% auto-rollback |
| Mean time to merge | <4 hours | >24h investigate |
| PR iteration count | ≤2 rounds | >3 rounds investigate process |
| Deploy frequency | Multiple/day | <1/week investigate blockers |

---

*This document is a living spec. Update it as we learn what works and what doesn't.*
*Last updated: 2026-02-05*
