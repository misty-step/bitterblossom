# Vision

## One-Liner
Bitterblossom is an autonomous software factory — the assembly line that lets supervisor agents spawn, manage, and iterate on high-quality AI sprites.

## North Star
A stable, resilient, extensible sprite factory where a high-level orchestrator (Opus 4.6 via OpenClaw/Pi) can declaratively spin up specialized sprites on Fly.io, assign them tasks via Ralph loops, and scientifically iterate on which configs/personas perform best for which tasks. Each sprite is constructive-only — opens PRs, never merges — with judgment left to the supervisor.

## Key Differentiators
- **Declarative sprite provisioning** — config-as-code for AI agent fleets
- **Scientific config iteration** — A/B test sprite configs against identical tasks, grade results
- **Constructive-only agents** — sprites open PRs but can't merge or destroy; supervisor retains judgment
- **Model-agnostic harness** — Claude Code harness today, but pluggable (Kimi Code 2.5 thinking via Moonshot for cost efficiency)
- **Beyond coding** — sprites for growth hacking, marketing, design, not just engineering

## Target User
An orchestrator agent (or human) that needs to manage a fleet of specialized AI workers. The primary interface is another AI (Opus 4.6), not a human dashboard.

## Current Focus
Exploratory/experimental phase. Priorities:
1. Stable, resilient sprite lifecycle (provision → sync → dispatch → teardown)
2. Strong base configs and hooks that produce effective Ralph loops
3. Safe GitHub credential sharing (bot accounts per sprite, audit trail)
4. Experimentability — easy to tweak configs and compare results
5. Extensibility — non-coding personas (growth, marketing, design)

## Open Questions
- Opinionated defaults vs. fully configurable by users?
- GitHub bot account provisioning strategy
- Evaluation framework for config A/B testing
- OpenClaw routing intelligence vs. explicit task assignment

---
*Last updated: 2026-02-05*
*Updated during: /groom session*
