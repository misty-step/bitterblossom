---
name: gather-pr-context
description: Gather the issue, PR intent, review state, and earlier fixer attempts before changing code.
---

# /gather-pr-context

Build the minimum context Thorn needs before touching code.

## Required Inputs

- The active PR number and branch
- The linked issue, if any
- The current CI failure

## Subagents

- `issue-reader`: fetch the linked issue title, body, acceptance criteria, and boundaries
- `pr-reader`: fetch the PR body, title, labels, and changed-file summary
- `review-reader`: fetch recent review comments and unresolved threads
- `history-reader`: fetch earlier fixer attempts from conductor events or recent branch history

## Output

Return a short structured note with:

1. Linked issue intent and acceptance criteria
2. PR intent and trade-offs
3. Diff summary and likely touch points
4. Review state and unresolved concerns
5. Previous fixer attempts and what they changed
6. Invariants that must not be broken
