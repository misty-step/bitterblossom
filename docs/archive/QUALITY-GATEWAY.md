# Quality Gateway — System Design for the Autonomous Software Factory

*Last updated: 2026-02-05*

## Philosophy

**Unix philosophy applied to quality.** Small, composable, well-tested tools. Each does one thing well. Together they form an impenetrable quality barrier. Every tool is version-controlled, documented, and independently testable.

**The faster we ship, the stronger the gates.** An autonomous factory producing code 24/7 needs more safeguards than a human team, not fewer. The goal: make it *structurally difficult* to ship bugs, and *trivially easy* to catch and fix them.

---

## System Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                    EVENT SOURCES                             │
│  GitHub (PRs, CI, pushes) · Fly.io (deploys, health)       │
│  Sentry (errors, perf) · Sprites (dispatch, completion)     │
└────────────┬────────────────────────┬───────────────────────┘
             │                        │
     ┌───────▼────────┐    ┌─────────▼──────────┐
     │  GitHub Actions │    │  Claw Cron Jobs     │
     │  (CI/CD layer)  │    │  (polling layer)    │
     │                 │    │                     │
     │ • Build & Test  │    │ • pr-shepherd       │
     │ • Lint & Type   │    │ • sentry-watcher    │
     │ • Coverage Gate │    │ • deploy-monitor    │
     │ • Cerberus      │    │ • fleet-health      │
     │ • Deploy Gate   │    │                     │
     └───────┬─────────┘    └─────────┬───────────┘
             │                        │
     ┌───────▼────────────────────────▼───────────┐
     │              CLAW (Coordinator)              │
     │  OpenClaw main session on Phaedrus's Mac    │
     │                                              │
     │  Receives: cron reports, Telegram messages   │
     │  Actions: dispatch sprites, Codex review,    │
     │           merge PRs, rollback deploys,       │
     │           alert Phaedrus                     │
     └───────┬─────────────────────────┬───────────┘
             │                         │
     ┌───────▼────────┐      ┌────────▼──────────┐
     │  Sprites        │      │  Phaedrus          │
     │  (executors)    │      │  (via Telegram)    │
     │  Fix, build,    │      │  Alerts, reports,  │
     │  test, PR       │      │  approvals         │
     └────────────────┘      └───────────────────┘
```

### Architecture Evolution

**v1 (historical):** Shell scripts + cron jobs for polling. Simple, worked well for bootstrapping.

**v2 (current):** Go control plane (`bb` CLI) handles fleet lifecycle, dispatch, and health monitoring. Polling scripts are being replaced by `bb watchdog` (fleet health) and `bb watch` (event stream monitoring). See [docs/MIGRATION.md](MIGRATION.md) for the shell→Go mapping.

**Polling vs webhooks:** Polling remains the pragmatic default for 15-minute resolution. `bb watchdog` replaces the ad-hoc shell health checks with a deterministic state machine. When sub-minute latency matters, a minimal webhook receiver can convert events to `bb` commands.

---

## The Tools (Unix-Style, Composable)

Each tool is a standalone script. Each can be run manually or via cron. Each outputs structured data (JSON or plain text). Each is independently testable.

### 1. `quality-spec.yml` — Declarative Quality Standard

A YAML file defining the quality bar for each project type. Lives in Bitterblossom, referenced by all other tools.

```yaml
# bitterblossom/quality/spec.yml
version: 1

profiles:
  typescript:
    ci:
      required_checks:
        - build
        - test
        - lint
        - type-check
        - cerberus
      coverage_threshold: 80
      warnings_as_errors: true
    linting:
      tool: eslint
      config: strict  # references bitterblossom/quality/configs/eslint-strict.json
    typing:
      strict: true
      noUncheckedIndexedAccess: true
      exactOptionalPropertyTypes: true
    branch_protection:
      require_pr: true
      require_checks: true
      require_reviews: 0  # Cerberus handles review — human review optional
      block_force_push: true
      block_deletions: true
    monitoring:
      sentry: required
      health_check: required
      structured_logging: required

  swift:
    ci:
      required_checks:
        - build-and-test
        - cerberus
      coverage_threshold: 80
      warnings_as_errors: true
    linting:
      tool: swiftlint
      config: strict  # references bitterblossom/quality/configs/.swiftlint-strict.yml
    branch_protection:
      require_pr: true
      require_checks: true
      require_reviews: 0
      block_force_push: true
    monitoring:
      sentry: optional  # macOS app, harder to instrument
      structured_logging: required

  static-site:
    ci:
      required_checks:
        - build
      coverage_threshold: 0
    branch_protection:
      require_pr: false  # Simple sites, fast iteration
```

### 2. `quality-audit` — Check a Repo Against Its Spec

```bash
#!/bin/bash
# bitterblossom/scripts/quality-audit.sh
# Usage: quality-audit.sh <org/repo> <profile>
# Output: JSON report of what passes and what's missing

REPO=$1
PROFILE=$2

# Check: Has CI workflow?
# Check: CI has required checks?
# Check: Branch protection configured?
# Check: Linting config present and strict?
# Check: Coverage above threshold?
# Check: Sentry configured?
# Check: CLAUDE.md present?
# Check: Tests exist?

# Output:
# { "repo": "misty-step/vox", "profile": "swift",
#   "checks": [
#     {"name": "ci_workflow", "status": "pass"},
#     {"name": "branch_protection", "status": "fail", "detail": "no protection on master"},
#     {"name": "coverage", "status": "warn", "detail": "62% < 80% threshold"},
#     ...
#   ],
#   "score": "6/10", "grade": "C" }
```

### 3. `pr-shepherd` — Monitor and Act on Open PRs

```bash
#!/bin/bash
# bitterblossom/scripts/pr-shepherd.sh
# Usage: pr-shepherd.sh
# Checks all open sprite PRs, reports status, optionally dispatches fixes

# For each open PR by configured sprite author(s):
#   1. Check CI status (passing/failing/pending)
#   2. Check review status (approved/changes_requested/pending)
#   3. Check age (>24h without activity = stale)
#   4. If CI failing: extract errors, generate fix prompt
#   5. If reviews need addressing: extract comments, generate fix prompt
#   6. If merge-ready: flag for Claw final review

# Output: JSON summary
# [
#   {"pr": 155, "repo": "vox", "ci": "failing", "reviews": "changes_requested",
#    "age_hours": 4, "action_needed": "fix_ci", "fix_prompt": "..."},
#   {"pr": 156, "repo": "vox", "ci": "passing", "reviews": "approved",
#    "age_hours": 4, "action_needed": "merge_review"}
# ]
```

### 4. `sentry-watcher` — Poll Sentry for Anomalies

```bash
#!/bin/bash
# bitterblossom/scripts/sentry-watcher.sh
# Usage: sentry-watcher.sh
# Polls Sentry API, reports new/spiking issues

# For each project in misty-step org:
#   1. Get unresolved issues sorted by date
#   2. Compare against last check (stored in /tmp/sentry-state.json)
#   3. Flag: new issues since last check
#   4. Flag: issues with >5% event rate increase
#   5. Flag: issues with >100 events in last hour

# Output: JSON alert list
# [
#   {"project": "heartbeat", "severity": "critical",
#    "issue": "TypeError: Cannot read property 'id' of undefined",
#    "events_1h": 150, "action": "investigate"}
# ]
```

### 5. `deploy-monitor` — Check Deployed App Health

```bash
#!/bin/bash
# bitterblossom/scripts/deploy-monitor.sh
# Usage: deploy-monitor.sh
# Checks health of all Fly.io apps

# For each Fly.io app in misty-step:
#   1. Check machine status (running/stopped/failed)
#   2. Hit health endpoint
#   3. Check recent deploy status
#   4. Check for restarts/crashes in last hour

# Output: JSON health report
```

### 6. `repo-scaffold` — Set Up a New Repo with Standards

```bash
#!/bin/bash
# bitterblossom/scripts/repo-scaffold.sh
# Usage: repo-scaffold.sh <name> <profile> [--private]
# Creates a new repo with full quality infrastructure

# 1. Create GitHub repo
# 2. Initialize with: README.md, CLAUDE.md, .gitignore, LICENSE
# 3. Copy CI workflow from template (profile-specific)
# 4. Copy Cerberus workflow + config
# 5. Copy linting config (profile-specific)
# 6. Set up branch protection rules
# 7. Run quality-audit to verify
```

---

## Where Things Live

### In Bitterblossom (version-controlled, shared with sprites)
```
bitterblossom/
├── quality/
│   ├── spec.yml                    # Declarative quality standards
│   └── configs/
│       ├── eslint-strict.json      # Shared ESLint config
│       ├── .swiftlint-strict.yml   # Shared SwiftLint config
│       ├── tsconfig-strict.json    # Shared TypeScript config
│       └── ci-templates/
│           ├── typescript.yml      # CI workflow template for TS
│           ├── swift.yml           # CI workflow template for Swift
│           └── cerberus.yml        # Cerberus workflow (copy from cerberus repo)
├── scripts/
│   ├── quality-audit.sh
│   ├── pr-shepherd.sh
│   ├── sentry-watcher.sh
│   ├── deploy-monitor.sh
│   └── repo-scaffold.sh
├── base/
│   └── prompts/
│       └── ralph-loop-v2.md        # Self-healing PR template
└── docs/
    └── QUALITY-GATEWAY.md          # This document
```

### In Each Repo (applied by scaffold or manually)
```
repo/
├── .github/
│   ├── workflows/
│   │   ├── ci.yml                  # Build + test + lint + coverage
│   │   └── cerberus.yml           # AI review council (copied from cerberus/)
│   └── cerberus/                  # Cerberus config + agents + scripts
├── CLAUDE.md                      # Project spec for AI agents
├── .eslintrc.json / .swiftlint.yml  # Strict linting
└── tsconfig.json                  # Strict typing (if TS)
```

### On Claw's Machine (OpenClaw + bb CLI)
```
~/.openclaw/workspace/
├── HEARTBEAT.md                   # Periodic check triggers
└── scripts/                       # Claw-specific operational scripts

Go CLI commands (replacing cron jobs):
├── bb watchdog          # Fleet health: dead/stale/blocked detection + auto-recovery
├── bb watch             # Real-time event stream dashboard
├── bb compose status    # Composition drift detection
└── bb status            # Fleet overview
```

### On Phaedrus's Machine (Cerberus, manual overrides)
```
Cerberus repo → installed as GitHub Action on all repos
Manual override: /council override on any PR
GitHub org admin: rulesets, branch protection, secrets
```

---

## GitHub Configuration (Org-Wide)

### Option A: GitHub Rulesets (Preferred — org-level, applies to all repos)

Requires org admin access. Rulesets are the modern replacement for per-repo branch protection. You set them once, they apply to all matching repos.

```
Ruleset: "production-branches"
  Target: all repositories, default branch
  Rules:
    - Require pull request before merge
    - Require status checks to pass:
        - "Build & Test" (or profile-equivalent)
        - "Council Verdict" (Cerberus)
    - Block force pushes
    - Block deletions
    - Require linear history (squash merge)
```

**This is the #1 thing to set up.** One ruleset = every repo in the org gets branch protection. New repos automatically inherit it.

### Option B: Per-Repo Branch Protection (Fallback)

If rulesets aren't available or need per-repo customization, the `repo-scaffold` script sets branch protection via API:

```bash
gh api repos/misty-step/$REPO/branches/master/protection \
  -X PUT \
  -f required_status_checks='{"strict":true,"contexts":["Build & Test","Council Verdict"]}' \
  -f enforce_admins=true \
  -f required_pull_request_reviews='{"required_approving_review_count":0}' \
  -F restrictions=null
```

### Shared Workflows via `misty-step/.github` Repo

Create `misty-step/.github` repo with:
- Shared CI workflow templates
- Default community health files (issue templates, PR templates)
- Org-level CODEOWNERS patterns

Then repos can reference shared workflows:
```yaml
jobs:
  ci:
    uses: misty-step/.github/.github/workflows/ci-typescript.yml@main
```

This means: update CI once → all repos get the update.

---

## Scaling to New Repos (Including Sprite-Created)

### The `repo-scaffold` Script
Every new repo goes through scaffold. This is non-negotiable. The script:
1. Creates the repo with correct visibility
2. Applies the profile-appropriate CI, linting, typing configs
3. Copies Cerberus workflow + config
4. Sets branch protection
5. Runs quality-audit to verify everything is wired

### Sprite Dispatch Prompts Include Quality Standards
The Ralph Loop v2 prompt template includes:
- "Run the full test suite before pushing"
- "Ensure `swift build` / `npm run build` has zero warnings"
- "Ensure linting passes"
- Sprint never push code that doesn't compile

### Bitterblossom Composition Includes Quality Config
Each sprite composition references the quality spec. When a new sprite is provisioned, it gets the quality expectations as part of its base config.

---

## Monitoring & Observability

### What We Have Now
- **Sentry** on 12 projects (all Next.js) — API access confirmed
- **GitHub Actions CI** on ~24 repos
- **Cerberus** being set up (5 AI reviewers per PR)
- **Fly.io** for deployments (health checks built-in)

### What We Need

#### Phase 1: Polling-Based Monitoring (This Week)
- [ ] `pr-shepherd` cron — every 30 min, check all open sprite PRs
- [ ] `sentry-watcher` cron — every 15 min, check for new/spiking issues
- [ ] `deploy-monitor` cron — every 30 min, check Fly.io app health
- [ ] State files in `/tmp/` for diffing against last check

#### Phase 2: Structured Logging Standard (Next Week)
- [ ] Define log format: JSON with `{timestamp, level, service, message, context}`
- [ ] Add to all deployed apps
- [ ] Fly.io log drain → searchable store (Fly.io's built-in logging, or Axiom/Logtail)

#### Phase 3: Alerting Pipeline (When Latency Matters)
- [ ] Sentry webhook → tiny Fly.io app → OpenClaw wake event
- [ ] GitHub webhook → same app → wake event for failed deploys
- [ ] OR: GitHub Actions step that calls `openclaw gateway wake` on failure
- [ ] Auto-rollback: Fly.io deploy fails health check → previous release auto-restores

---

## Error Rate Auto-Rollback

### Design
```
Deploy completes
    │
    ▼
Health check passes? ──no──▶ Auto-rollback (Fly.io handles this)
    │ yes
    ▼
Monitor error rate for 10 minutes
    │
    ▼
Error rate > 5%? ──yes──▶ Rollback + alert Claw
    │ no
    ▼
Deploy confirmed stable
```

### Implementation
Fly.io already supports health check-based auto-rollback. For error rate monitoring, the `sentry-watcher` cron detects spikes and alerts. Manual rollback is:
```bash
flyctl releases --app $APP | head -5
flyctl deploy --image $PREVIOUS_IMAGE --app $APP
```

Later: automate this in a deploy-gate script.

---

## Coverage Enforcement

### TypeScript (Jest/Vitest)
```json
// package.json or vitest.config.ts
{
  "coverage": {
    "thresholds": {
      "lines": 80,
      "branches": 75,
      "functions": 80,
      "statements": 80
    }
  }
}
```
CI fails if coverage drops below threshold.

### Swift (Xcode/SPM)
```bash
swift test --enable-code-coverage
xcrun llvm-cov report .build/debug/VoxPackageTests.xctest/Contents/MacOS/VoxPackageTests \
  -instr-profile .build/debug/codecov/default.profdata
```
Parse coverage percentage in CI, fail if below 80%.

### Coverage Ratchet
Coverage threshold can only go up. When a repo hits 85%, the threshold moves to 85%. Implemented as: store current threshold in `.quality.yml` in the repo, CI reads it.

---

## Metrics & Tracking

| Metric | Source | Target | Alert |
|--------|--------|--------|-------|
| Test coverage | CI coverage report | >80% (>90% goal) | <80% blocks merge |
| CI pass rate | GitHub Actions API | >95% | <90% investigate |
| Error rate | Sentry API | <1% | >5% rollback |
| PR merge time | GitHub API | <4 hours | >24h stale alert |
| PR iteration count | GitHub API | ≤2 rounds | >3 rounds process review |
| Deploy frequency | Fly.io API | Multiple/day | <1/week investigate |
| Sprite dispatch success | Bitterblossom logs | >80% | <60% prompt review |
| Cerberus false positive rate | Manual tracking | <20% | >40% tune prompts |

---

## Implementation Priority

### Now (Today)
1. ~~Ralph Loop v2 prompt template~~ ✅
2. ~~Re-dispatch sprites with CI fix prompts~~ ✅
3. Cerberus setup (Phaedrus)

### This Week
4. `pr-shepherd.sh` — automated PR monitoring cron
5. `sentry-watcher.sh` — error anomaly detection cron
6. GitHub Rulesets — org-wide branch protection
7. `misty-step/.github` — shared workflow templates
8. Coverage enforcement in CI for vox + heartbeat

### Next Week
9. `quality-audit.sh` — repo compliance checker
10. `repo-scaffold.sh` — new repo bootstrap
11. SwiftLint strict config for Vox
12. ESLint strict config for TS projects
13. Structured logging standard

### Ongoing
14. Coverage ratchet implementation
15. Cerberus prompt tuning based on false positive analysis
16. Deploy-gate automation
17. Webhook alerting (when polling latency becomes a problem)

---

*This is a living document. Update as we learn what works.*
