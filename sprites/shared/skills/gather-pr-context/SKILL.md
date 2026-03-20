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

- `issue-reader`
  Fetch the linked issue title, body, acceptance criteria, and hard boundaries.
- `pr-reader`
  Fetch the PR title, full body, labels, changed files, and diff summary.
- `review-reader`
  Fetch recent review comments, unresolved threads, and any stated reviewer concerns.
- `history-reader`
  Fetch earlier fixer attempts from conductor events, recent commits, or prior fixer branches.

## Procedure

1. Read the issue first so the acceptance criteria anchor every later decision.
2. Read the PR description and diff summary before interpreting the failing check.
3. Read review state to avoid reintroducing an already-rejected approach.
4. Read earlier fixer attempts to learn what already failed or caused regressions.
5. Write down the invariants that must survive the fix before moving to `/diagnose-ci`.

## Output

Return a short structured note with:

1. Linked issue intent and acceptance criteria
2. PR intent, explicit trade-offs, and any scope boundaries in the PR body
3. Diff summary, likely touch points, and which files the failing checks exercise
4. Review state, unresolved concerns, and comments that affect the fix
5. Previous fixer attempts and what they changed
6. Invariants that must not be broken
