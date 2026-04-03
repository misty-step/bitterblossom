# Agent session longevity — fix premature budget exhaustion

Priority: critical
Status: ready
Estimate: M

## Goal
Agents should run for hours on a single session, not exhaust their budget in 10 minutes reading context files. Investigate and fix the harness/prompt configuration so Weaver completes at least one full backlog item per session.

## Problem
Factory audit runs 2-4: Weaver reads AGENTS.md, CLAUDE.md, project.md, WORKFLOW.md, backlog items, skill files, and test files — consuming its entire context/session budget before reaching the implementation phase. The agent never gets to ship code.

## Sequence
- [ ] Instrument: add token consumption logging to Codex dispatch. Record how many tokens the agent uses on context reading vs implementation.
- [ ] Check Codex session limits: what is the actual session timeout? Is it context window exhaustion or time-based? Read Codex docs for `--model`, `--yolo`, session lifetime configuration.
- [ ] Optimize Weaver AGENTS.md: reduce upfront context reading. The agent should read ONLY: its own AGENTS.md, the target backlog item, and the files it needs to modify. NOT project.md, NOT WORKFLOW.md, NOT MEMORY.md, NOT all backlog items.
- [ ] Test different `reasoning_effort` levels: `medium` (current) vs `low` for builder tasks. Lower effort = less token consumption per step.
- [ ] Test different models: `gpt-5.4` (current) vs `codex-5.3` or cheaper alternatives for build tasks.
- [ ] Consider a sprite-local cron/loop: instead of relying on the conductor to relaunch after session death, have the sprite itself restart the agent in a bash loop. This makes each sprite self-contained and self-healing for session budget limits.
- [ ] Test: deploy optimized config, run factory audit, verify Weaver completes a full build cycle.

## Oracle
- [ ] Weaver completes at least one backlog item per session (branch → implement → test → push → PR)
- [ ] Session runs for >30 minutes of productive work before any budget limit
- [ ] Token consumption is measurable and logged
