# Backlog Ideas

Last groomed: 2026-03-14

## High Potential (promote next session if capacity)

- **Phoenix.PubSub EventBus for real-time event broadcasting** — Prerequisite for dashboard and any future sidecar. Eliminates database polling for state changes. Source: #614, architecture critique.
- **Cost tracking with model, provider, and budget gates** — Every dispatch records model, provider, tokens, estimated cost. Budget gates pause orchestrator on threshold breach. Source: #616, groom.
- **OpenTelemetry GenAI spans for run telemetry** — Adopt OTEL semantic conventions for free Langfuse/Datadog/Arize integration without custom dashboards. Source: #619, groom.

- **Fast-fail patterns for common issue types** — Auth, deps, env setup should fail within 5 minutes, not 25. Issue classification drives timeout policy.

## Next Sprint: Multi-Repo Conductor

Added 2026-03-15. One conductor, one fleet, multiple repos. Sprites are roles (Weaver, Thorn, Fern, Muse), not repo bindings.

### Design Principles
- **Sprites are repo-agnostic.** A Weaver builds for any repo. A Thorn fixes CI in any repo. No `cerberus-builder` — just Weaver dispatched to Cerberus.
- **One conductor, 10-20 sprites.** The conductor manages the full fleet and routes work to available sprites by role, not by repo.
- **Repos declared in fleet.toml.** Each repo has its own label filter, issue selection, and calibration profile. Sprites declared separately from repos.

### fleet.toml sketch
```toml
[[repo]]
name = "misty-step/bitterblossom"
label = "autopilot"

[[repo]]
name = "misty-step/cerberus"
label = "autopilot"

[[repo]]
name = "misty-step/canary"
label = "autopilot"

[[repo]]
name = "misty-step/volume"
label = "autopilot"

[[sprite]]
name = "bb-weaver-1"
role = "builder"

[[sprite]]
name = "bb-weaver-2"
role = "builder"

[[sprite]]
name = "bb-thorn"
role = "fixer"

[[sprite]]
name = "bb-fern"
role = "polisher"

[[sprite]]
name = "bb-muse"
role = "muse"
```

### Key Changes
- Orchestrator polls issues across all declared repos, round-robin or priority-weighted
- Fixer/Fern watch PRs across all repos
- Merge loop merges across all repos
- Workspace module clones/manages multiple repos per sprite
- `bb setup` provisions all declared repos on each sprite
- Per-repo harness detection (scan CLAUDE.md, CI config, test runner) — see Adaptive Harness section below

### Prerequisites
- #680 (remove factory/ branch prefix) — sprites must work on any branch in any repo
- #675 (harness-agnostic artifact protocol) — can't assume Claude-specific conventions
- #676 (close issues after merge) — prevents re-leasing across repos
- #686 (Muse sprite) — reflection should span all repos

### Sprite Math
5 sprites (2 Weavers + 1 Thorn + 1 Fern + 1 Muse) can serve 4 repos. Scale by adding Weavers for throughput, not by adding per-repo sprites.

## Someday / Maybe

- **Promptfoo-based model comparison and regression testing** — Declarative LLM evals with CI/CD integration. Depends on behaviours (#613). Source: #620, groom.
- **Bakeoffs: dispatch same issue to multiple model/harness configs** — Multi-model comparison (Sonnet vs Opus vs Kimi vs GPT). Depends on behaviours (#613) + evals. Source: #617, groom.
- **Quality metric (score function) for sprite output** — Karpathy-style autoresearch: fixed metric decides what stays. No clear path yet. Source: #537, user.

## Strategic Vision: Multi-Workflow Sprites

Captured 2026-03-14. These are beyond the current sprint scope but represent the product direction.

### Workflow Types

1. **Backlog Clearing (current focus)** — Pull issues, assign, checkout, implement, simplify, review, open PR, fix CI, address comments, refactor, document, squash-merge, repeat until backlog is cleared.

2. **CI-Fix Sprites** — GitHub webhook-triggered. When CI completes with failures on a PR, sprite checks out the PR, investigates the failure, and ships a fix. Focused, reactive, fast cycle time.

3. **Auto-Triage Sprites** — Plugged into observability feed (Vigil webhooks). On error: systematic debug investigation, hypothesis formation, rigorous root-cause determination, strategic fix, tests, verify, open PR, merge, write postmortem, create follow-up issues to improve quality gates.

4. **Domain-Specialized Sprites** — Narrowly scoped environments tuned per workflow. Triage sprites get devops/sysadmin/infrastructure/debugging/monitoring skills, not frontend design. Backlog sprites get full-stack skills. Each environment is purpose-built.

### Design Questions

- How do we declare sprite specializations? (Config file? Labels? Separate profiles?)
- How does the conductor route work to the right sprite type?
- Should specialization be environment-level (tools/skills preloaded) or prompt-level (persona + constraints)?
- What's the minimum viable second workflow after backlog clearing?
- How do we avoid framework-ification and plugin proliferation? (Reference architecture survey warning)

### Rollout Strategy

- Phase 1: Get primary backlog-clearing workflow solid and trustworthy
- Phase 2: Add next most valuable workflow (likely CI-fix — tightest feedback loop)
- Phase 3: Auto-triage (requires self-hosted observability service — see ~/Development/vigil/)
- Phase 4: Domain specialization framework (only after 2-3 workflows prove the pattern)

## Strategic Vision: Adaptive Harness Engineering

Captured 2026-03-14. Informed by "Harness Engineering Is Cybernetics" (odysseus0z).

The conductor is a cybernetic governor. Its value is the feedback loop, not the code it dispatches. The critical design problem ahead: making the harness adaptive to arbitrary target repos, not just bitterblossom.

### The Calibration Surface

For each target repo, the conductor needs to detect and internalize:
- **What "good" means** — CLAUDE.md, conventions, architecture patterns, coding standards
- **What feedback loops exist** — tests, linters, CI, type checkers, pre-commit hooks
- **What the definition of done is** — merge policy, review requirements, quality gates
- **What knowledge is missing** — gaps in docs, untested paths, undocumented invariants

Today this calibration is manual (read CLAUDE.md, read project.md). The vision: the conductor auto-detects the harness on first contact with a repo, builds a calibration profile, and adapts its builder prompts, governance policy, and retro analysis accordingly.

### Per-Repo Harness Detection

When the conductor is pointed at a new repo for the first time:
1. **Scan** — detect language, framework, test runner, CI config, CLAUDE.md/AGENTS.md, project structure
2. **Profile** — build a harness profile: what feedback loops exist, what's fast vs. slow, what gates are mechanical vs. judgment
3. **Calibrate** — inject the profile into builder prompts and governance policy
4. **Learn** — retro loop updates the profile as runs reveal gaps (tests that don't exist, conventions that aren't documented)

### Harness Quality as a Metric

The conductor should be able to assess a repo's harness quality:
- Does it have tests? How fast? What coverage?
- Does it have CI? Does CI actually catch real issues?
- Does it have documentation agents can read?
- Are there architectural constraints encoded mechanically (types, linters) or only in prose?

Low harness quality → conductor invests in harness improvement before feature work (write tests, add CI, create CLAUDE.md). High harness quality → conductor can clear backlog at speed.

### The Drift Trap

Without codified constraints, agents amplify drift at machine speed. The retro loop is the anti-drift sensor. But it only works if the retro agent knows what "clean" looks like for THIS repo. Per-repo calibration is what makes the retro loop useful across diverse projects.

## Research Prompts

- **Sprite specialization patterns** — How do other agent orchestrators handle multi-workflow dispatch? Is it profile-based, environment-based, or routing-based?
- **Webhook-driven reactive agents** — Best practices for GitHub webhook → agent dispatch. Latency, deduplication, idempotency.
- **Error-feed → autonomous fix pipelines** — Prior art on auto-triage systems. How do they determine when to fix vs. escalate?
- **Auto-detecting repo harness quality** — Can we score a repo's readiness for agent work? What signals matter most? (test coverage, CI config, doc quality, type strictness)
- **Cybernetic calibration for multi-repo orchestration** — How should per-repo harness profiles be structured? What's the minimum viable profile? When should the conductor invest in improving a repo's harness vs. working within its constraints?

## Archived This Session

(none — first grooming session with BACKLOG.md)
