# Task

{{TASK_DESCRIPTION}}

# Instructions

You are working autonomously. Do NOT stop to ask clarifying questions.
If something is ambiguous, make your best judgment call and document the decision.

## Workflow

### Phase 1: Implementation
1. Read MEMORY.md and CLAUDE.md for context from previous iterations
2. Assess current state: what's done, what's left, what's broken
3. Work on the highest-priority remaining item
4. Run tests after every meaningful change
5. Commit working changes with descriptive messages (conventional commits)
6. Push frequently — don't accumulate uncommitted work

### Phase 2: PR and CI
7. When implementation is complete, open a PR if you haven't already:
   `gh pr create --title "<type>: <description>" --body "<details>"`
8. After opening or pushing to the PR, check CI status:
   `gh pr checks $(gh pr view --json number --jq .number) --watch --fail-fast`
   Or poll: `gh pr checks $(gh pr view --json number --jq .number)`
9. If CI fails: read the failure logs, fix the issue, commit, push
10. Repeat steps 8-9 until CI is green

### Phase 3: Review Response
11. Check for review comments:
    `gh pr view --json reviews,comments --jq '.reviews[] | "\(.state): \(.body)"'`
    `gh api repos/{{REPO}}/pulls/$(gh pr view --json number --jq .number)/comments --jq '.[] | "\(.path):\(.line) \(.body)"'`
12. If there are review comments requesting changes: address them, commit, push
13. Repeat steps 8-12 until CI is green and no pending review requests

### Completion
14. When PR is green and either approved or no pending reviews:
    Create a file named exactly TASK_COMPLETE (no file extension) with a summary of what was done.
    Do NOT use TASK_COMPLETE.md — the detection system expects the extensionless filename.
15. Update MEMORY.md with learnings from this task

## When you get stuck
- If genuinely blocked (missing credentials, permission error, external dependency):
  Write BLOCKED.md explaining exactly what you need, then stop
- If CI keeps failing on the same issue after 3 attempts:
  Write BLOCKED.md with the persistent failure details
- Otherwise: KEEP WORKING. Don't stop for cosmetic concerns or hypothetical questions.

## Git workflow
- Work on a feature branch (never main/master)
- Commit frequently with conventional commit messages
- Include `Co-Authored-By: {{SPRITE_NAME}} <noreply@anthropic.com>` in commits
- Push to origin after each meaningful commit
- Open PR early (can be draft) so CI runs sooner
