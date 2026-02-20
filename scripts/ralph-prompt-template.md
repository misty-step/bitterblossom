# Task

{{TASK_DESCRIPTION}}

# Instructions

You are working autonomously. Do NOT stop to ask clarifying questions.
If something is ambiguous, make your best judgment call and document the decision.

## Workflow

### 1) Implement
1. Read `MEMORY.md` and `CLAUDE.md`.
2. Implement the issue directly.
3. Run relevant tests/lint after meaningful changes.
4. Commit and push with conventional commits.

### 2) Open PR
1. Open or update a PR for your branch:
   `gh pr create --title "<type>: <description>" --body "<details>"`
2. Include a clear summary of files changed and verification run.

### 3) Complete (do not wait in long loops)
1. Do NOT wait for full CI completion or review rounds in this run.
2. Do NOT run long polling loops (`sleep 300`, repeated `gh pr checks`, etc.).
3. As soon as implementation is pushed and PR exists, write `TASK_COMPLETE` with:
   - branch name
   - PR URL
   - tests run
   - short summary of changes
4. `TASK_COMPLETE` must be extensionless. Do NOT use `TASK_COMPLETE.md`.

### 4) Blocked path
If you cannot proceed (missing credentials, hard permission errors, repeated reproducible infra failures), write `BLOCKED.md` with exact blocker details and stop.

## Git
- Work on a feature branch (never main/master)
- Commit frequently with conventional commit messages
- Include `Co-Authored-By: {{SPRITE_NAME}} <noreply@anthropic.com>` in commits
- Push to origin after each meaningful commit
