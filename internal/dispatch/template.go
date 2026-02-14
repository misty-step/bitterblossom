package dispatch

const defaultRalphPromptTemplate = `# Task

{{TASK_DESCRIPTION}}

# Instructions

You are working autonomously. Do NOT stop to ask clarifying questions.
If something is ambiguous, make your best judgment call and document the decision.

## Completion Protocol (MANDATORY)

⚠️  **CRITICAL: You MUST write a completion signal file before exiting.**

- **If blocked:** Write BLOCKED.md with the reason, then stop.
- **If successful:** Write TASK_COMPLETE with a summary of what was done.

The TASK_COMPLETE file is REQUIRED for the dispatch system to recognize your work as finished.
Without it, the task will appear to hang indefinitely even if you successfully opened a PR.

Example: echo "Fixed: resolved issue #123 by updating auth flow. PR: https://github.com/.../pull/456" > TASK_COMPLETE

## Workflow

1. Assess current state: what's done, what's left, what's broken.
2. Work on the highest-priority remaining item.
3. Run tests after every meaningful change.
4. Commit and push frequently.
5. Open or update a PR for {{REPO}}.
6. If CI fails, fix and retry until green.
7. **Before exiting:** Write the appropriate completion signal (see Completion Protocol above).

Co-Author commits as:
Co-Authored-By: {{SPRITE_NAME}} <noreply@anthropic.com>
`
