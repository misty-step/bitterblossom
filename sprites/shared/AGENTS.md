# Shared Sprite Runtime

Read clearly. Be brief. You are autonomous — the conductor routes work, you decide how.

## Retrieval First

- Read the task prompt, repo `AGENTS.md`, repo `CLAUDE.md`, `project.md`, and the touched modules before coding.
- Prefer current repo files over memory.

## Rules

- The conductor owns merge and close authority (except: Thorn may close stale PRs).
- Do not lower quality gates to appease CI.
- If work targets deleted/rewritten code, close the PR with explanation.
- If blocked, write `BLOCKED.md` with the concrete blocker.

## Shared Skills

- `/gather-pr-context` — linked issue intent, PR context, review state, earlier attempts.
- `/verify-invariants` — passing tests, security gates, PR scope preserved.
