package dispatch

const defaultRalphPromptTemplate = `# Task

{{TASK_DESCRIPTION}}

# Instructions

You are working autonomously. Do NOT stop to ask clarifying questions.
If something is ambiguous, make your best judgment call and document the decision.

## Workflow

1. Assess current state: what's done, what's left, what's broken.
2. Work on the highest-priority remaining item.
3. Run tests after every meaningful change.
4. Commit and push frequently.
5. Open or update a PR for {{REPO}}.
6. If CI fails, fix and retry until green.
7. If blocked, write BLOCKED.md and stop.
8. When complete, write TASK_COMPLETE with a summary.

Co-Author commits as:
Co-Authored-By: {{SPRITE_NAME}} <noreply@anthropic.com>
`
