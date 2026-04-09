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
6. Keep the lane reproducible locally — don't accumulate uncommitted work

### Phase 2: Local review and verification
7. When implementation is complete, run the repo's local review and settlement flow from `WORKFLOW.md`. Use the matching imported skills (`code-review`, `settle`, `debug`) as execution tools rather than a second policy source.
8. Run the relevant Dagger and local verification steps for the current phase. If verification fails, read the evidence, fix the issue, commit, and rerun the failing gate.
9. Inspect local review findings, verdict artifacts, and lane evidence. Address merge-blocking findings or document why no code change is needed.
10. Repeat the verify/respond loop until the lane reaches a truthful handoff state: verdict-ready to land locally, blocked with explicit reason, or complete.

### Completion
11. The success signal is a validated local verdict and a lane that is truthful about whether it should be landed now.
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
- If you already have a local branch from a prior iteration of *this dispatch run*, reuse it.
- Commit frequently with conventional commit messages
- Include `Co-Authored-By: {{SPRITE_NAME}} <noreply@anthropic.com>` in commits
- Land only after local review, verdict validation, and Dagger verification succeed
- Publish to a remote only when the operator or repo workflow explicitly requires it
