# Agent session longevity — fix premature budget exhaustion

Priority: critical
Status: ready
Estimate: M

## Goal
Agents should run for hours on a single session, not exhaust their budget in 10 minutes reading context files. Investigate why Codex sessions end prematurely and fix the harness/prompt configuration.

## Problem
Factory audit runs 2-4: Weaver reads AGENTS.md, CLAUDE.md, project.md, WORKFLOW.md, backlog.d/ items, skill files, and test files — consuming its entire context/session budget before reaching the implementation phase. The agent never gets to code. This is NOT an infrastructure problem — it's a harness configuration problem.

## Investigation areas
- Codex session timeout vs context window exhaustion — which limit is hit first?
- Model reasoning_effort setting (`medium` in fleet.toml) — does higher effort consume budget faster?
- Prompt size — is the loop prompt too large? Are too many files being read?
- Model selection — is gpt-5.4 the right model for a long-running build loop?
- Comparison: how do Symphony/Sandcastle configure their agent sessions for multi-hour runs?

## Sequence
- [ ] Instrument a Codex session to measure actual token consumption vs limits
- [ ] Compare Codex session docs for session timeout, context limits, and best practices
- [ ] Optimize Weaver's AGENTS.md to minimize context consumption (fewer files read upfront)
- [ ] Test different models (Codex 5.3, GPT 5.4 Mini) for build tasks
- [ ] Consider a taskmaster/cron pattern on sprites for session restart without conductor intervention

## Oracle
- [ ] Weaver completes at least one backlog item per session (branch → implement → test → push → PR)
- [ ] Session runs for >30 minutes before context budget is exhausted
