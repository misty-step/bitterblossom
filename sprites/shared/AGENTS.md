# Shared Sprite Runtime

Read clearly. Be brief.

## Retrieval First

- Read the task prompt, repo `AGENTS.md`, repo `CLAUDE.md`, `project.md`, and the touched modules before coding.
- Prefer current repo files over memory.

## Rules

- The conductor owns merge and close authority.
- Do not lower quality gates to appease CI.
- If blocked, write `BLOCKED.md` with the concrete blocker.

## Shared Skills

- `/gather-pr-context` gathers linked issue intent, PR context, review state, and earlier fixer attempts before code changes.
- `/verify-invariants` checks that passing tests, security gates, and PR scope were preserved after the change.
