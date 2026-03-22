# Backlog Ideas

Last groomed: 2026-03-16

## High Potential (promote next session if capacity)

- **Muse sprite — deliberative reflection + daily synthesis** — Replace per-run retro with a dedicated reflection sprite that synthesizes across runs. Demoted from #686: good idea, but simplification sprint comes first. Source: #686, groom.
- **Bounded adversarial review before opening a PR** — Run a review agent against the builder's work before the PR goes public. Catches issues earlier in the loop. Demoted from #506. Source: #506, groom.
- **Fast-fail patterns for common issue types** — Auth, deps, env setup should fail within 5 minutes, not 25. Issue classification drives timeout policy. Source: 2026-03-14 groom.

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

### Prerequisites (from active backlog)
- #680 (remove factory/ branch prefix) — sprites must work on any branch in any repo
- #675 (delete artifact protocol) — can't assume Claude-specific conventions
- #676 (close issues after merge) — prevents re-leasing across repos

### Sprite Math
5 sprites (2 Weavers + 1 Thorn + 1 Fern + 1 Muse) can serve 4 repos. Scale by adding Weavers for throughput, not by adding per-repo sprites.

- **Early builder validation checkpoints** — If a builder run will ultimately produce no PR, detect this within 2-3 minutes rather than running full build cycles. Validate issue feasibility, code access, and basic setup before deep work.

## Someday / Maybe

- **Promptfoo-based model comparison and regression testing** — Declarative LLM evals with CI/CD integration. Depends on behaviours (#613, closed). Source: #620, groom.
- **Bakeoffs: dispatch same issue to multiple model/harness configs** — Multi-model comparison (Sonnet vs Opus vs Kimi vs GPT). Depends on behaviours (#613, closed) + evals. Source: #617, groom.
- **Quality metric (score function) for sprite output** — Karpathy-style autoresearch: fixed metric decides what stays. No clear path yet. Source: #537, user.

## Strategic Vision: Multi-Workflow Sprites

Captured 2026-03-14. Beyond current sprint scope but represent the product direction.

### Workflow Types

1. **Backlog Clearing (current focus)** — Pull issues, assign, checkout, implement, simplify, review, open PR, fix CI, address comments, refactor, document, squash-merge, repeat.

2. **CI-Fix Sprites** — GitHub webhook-triggered. When CI fails on a PR, sprite checks out, investigates, ships a fix. Focused, reactive, fast cycle time.

3. **Auto-Triage Sprites** — Plugged into observability feed (Vigil webhooks). On error: debug, root-cause, fix, test, PR, merge, postmortem, follow-up issues.

4. **Domain-Specialized Sprites** — Narrowly scoped environments tuned per workflow. Triage sprites get devops/debugging skills, not frontend design.

### Rollout Strategy

- Phase 1: Get primary backlog-clearing workflow solid and trustworthy
- Phase 2: Add CI-fix workflow (tightest feedback loop)
- Phase 3: Auto-triage (requires Vigil)
- Phase 4: Domain specialization (only after 2-3 workflows prove the pattern)

## Strategic Vision: Adaptive Harness Engineering

Captured 2026-03-14. Informed by "Harness Engineering Is Cybernetics" (odysseus0z).

The conductor is a cybernetic governor. Its value is the feedback loop, not the code it dispatches. Critical design problem: making the harness adaptive to arbitrary target repos.

### Per-Repo Harness Detection
1. **Scan** — detect language, framework, test runner, CI config, CLAUDE.md/AGENTS.md
2. **Profile** — what feedback loops exist, fast vs. slow, mechanical vs. judgment gates
3. **Calibrate** — inject into builder prompts and governance policy
4. **Learn** — retro loop updates profile as runs reveal gaps

## Research Prompts

- **Sprite specialization patterns** — How do other agent orchestrators handle multi-workflow dispatch?
- **Webhook-driven reactive agents** — GitHub webhook → agent dispatch best practices.
- **Error-feed → autonomous fix pipelines** — Prior art on auto-triage. When to fix vs. escalate?
- **Auto-detecting repo harness quality** — Score a repo's readiness for agent work.
- **Cybernetic calibration for multi-repo orchestration** — Per-repo harness profile structure.

## Archived This Session

- ~~Phoenix.PubSub EventBus~~ — done (#614, closed)
- ~~Cost tracking with budget gates~~ — done (#616, closed)
- ~~OpenTelemetry GenAI spans~~ — done (#619, closed)
- ~~PR Shepherding~~ — implemented via Fixer + Polisher GenServers (#515, closed)
