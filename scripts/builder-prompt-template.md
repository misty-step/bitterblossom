# Task

{{TASK_DESCRIPTION}}

# Instructions

You are working autonomously. Do NOT stop to ask clarifying questions.
If something is ambiguous, make your best judgment call and document the decision.

## Workflow

### Phase 1: Implementation
1. Read MEMORY.md, CLAUDE.md, and repo WORKFLOW.md for context from previous iterations
2. Assess current state: what's done, what's left, what's broken
3. Work on the highest-priority remaining item
4. Run tests after every meaningful change
5. Commit working changes with descriptive messages (conventional commits)
6. Push frequently — don't accumulate uncommitted work

### Phase 2: PR and verification
7. When implementation is complete, create or update the PR under repo `WORKFLOW.md`, using the matching imported skills (`pr`, `pr-fix`, `pr-polish`, `debug`) as execution tools rather than a second policy source.
8. Run the relevant checks for the current phase. If CI or local verification fails, read the evidence, fix the issue, commit, and push.
9. Inspect review comments and review threads. Classify them semantically against repo `WORKFLOW.md`, then address active merge-blocking findings or document why no code change is needed.
10. Repeat the verify/respond loop until the lane reaches a truthful handoff state: ready for review, blocked with explicit reason, or complete.

### Completion
11. The success signal is an open PR on the assigned `factory/*` branch.
12. Create a file named exactly TASK_COMPLETE (no file extension) with a summary of what was done.
    Do NOT use TASK_COMPLETE.md — the detection system expects the extensionless filename.
13. Update MEMORY.md with learnings from this task

## When you get stuck
- If genuinely blocked (missing credentials, permission error, external dependency):
  Write BLOCKED.md explaining exactly what you need, then stop
- If CI keeps failing on the same issue after 3 attempts:
  Write BLOCKED.md with the persistent failure details
- Otherwise: KEEP WORKING. Don't stop for cosmetic concerns or hypothetical questions.

## Git workflow
- Work on a feature branch (never main/master)
- **Branch naming**: append a timestamp to avoid collisions with prior dispatch runs:
  `git checkout -b fix/NNN-short-description-$(date +%Y%m%d-%H%M)`
  Example: `fix/406-timeout-grace-20260220-1730`
- **Before pushing**: use `--force-with-lease` on non-fast-forward failure:
  ```bash
  git push -u origin <branch-name> || git push --force-with-lease -u origin <branch-name>
  ```
  **Never rebase** — repo policy hooks (`destructive-command-guard.py`) block it.
- If you already have a local branch from a prior iteration of *this dispatch run*, reuse it.
- Commit frequently with conventional commit messages
- Include `Co-Authored-By: {{SPRITE_NAME}} <noreply@anthropic.com>` in commits
- Push to origin after each meaningful commit
- Open PR early (can be draft) so CI runs sooner
