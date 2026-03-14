# Backlog Ideas

Last groomed: 2026-03-14

## High Potential (promote next session if capacity)

- **Phoenix.PubSub EventBus for real-time event broadcasting** — Prerequisite for dashboard and any future sidecar. Eliminates database polling for state changes. Source: #614, architecture critique.
- **Cost tracking with model, provider, and budget gates** — Every dispatch records model, provider, tokens, estimated cost. Budget gates pause orchestrator on threshold breach. Source: #616, groom.
- **OpenTelemetry GenAI spans for run telemetry** — Adopt OTEL semantic conventions for free Langfuse/Datadog/Arize integration without custom dashboards. Source: #619, groom.

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

## Research Prompts

- **Sprite specialization patterns** — How do other agent orchestrators handle multi-workflow dispatch? Is it profile-based, environment-based, or routing-based?
- **Webhook-driven reactive agents** — Best practices for GitHub webhook → agent dispatch. Latency, deduplication, idempotency.
- **Error-feed → autonomous fix pipelines** — Prior art on auto-triage systems. How do they determine when to fix vs. escalate?

## Archived This Session

(none — first grooming session with BACKLOG.md)
