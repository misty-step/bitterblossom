# Vision

## One-Liner

Bitterblossom dispatches coding tasks to persistent AI sprites. Four core commands, ~800 LOC Go, one small bash loop.

## North Star

Reliable dispatch. An operator (human or agent) says "sprite X, do Y on repo Z" and gets back a PR or a clear explanation of why not. The ralph loop runs the agent, checks for completion signals, enforces time/iteration limits, and exits with a meaningful code. Everything else is plumbing.

## Key Differentiators

- **Small LOC is the feature.** Thin deterministic CLI for transport, thick Claude Code skills for intelligence. Complexity lives where it can be iterated cheaply (skills), not where it's expensive to change (compiled Go).
- **Single-path runtime.** Claude Code + Sonnet 4.6 + official `ralph-loop` plugin is the only production dispatch path. Reduced branching improves reliability and operability.
- **Constructive-only agents.** Sprites open PRs but never merge or destroy. Judgment stays with the operator.
- **Persistent environments.** Sprites auto-sleep at near-zero cost, wake instantly. Setup once, dispatch forever.

## Target User

An operator (human or coding agent) dispatching tasks to sprites. The primary interface is `bb dispatch` from a terminal or CI pipeline.

## Current Focus

Dispatch reliability. The rewrite achieved architectural simplicity; now we harden the pipeline:
1. 5/5 dispatch success rate on warm sprites
2. Clear signal protocol (TASK_COMPLETE, BLOCKED.md)
3. Accurate status reporting with connectivity verification
4. Ralph loop resilience (iteration limits, timeouts, stale process cleanup)

## Non-Goals

These were explored in v1 and intentionally dropped:
- Fleet composition YAML reconciliation
- A/B sprite configuration testing
- Event streaming / JSONL dashboards
- Watchdog auto-recovery
- On-sprite agent supervisor daemon

If any of these become needed again, they'll be implemented as skills, not Go code.

---
*Last updated: 2026-02-16*
*Updated during: SDK v2 rewrite codification*
