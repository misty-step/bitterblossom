---
name: resolve-conflict
description: Rebase a PR onto the base branch, resolve conflicts, or close stale PRs.
---

# /resolve-conflict

Resolve merge conflicts or close PRs that target fundamentally rewritten code.

## Steps

1. `git fetch origin`
2. Identify the default branch: `git symbolic-ref refs/remotes/origin/HEAD | sed 's|refs/remotes/origin/||'`
3. `git rebase origin/$default_branch`

## If rebase succeeds

Push: `git push --force-with-lease`

## If rebase has conflicts

Examine each conflicting file:

- **File still exists on base branch, changes are mechanical** (moved code, renamed imports, formatting) → resolve the conflict, continue rebase.
- **File was deleted or fundamentally rewritten on base branch** → the PR is stale. Abort the rebase and close the PR.

## Closing a stale PR

When closing, leave a comment explaining:
- Which files were deleted or rewritten on the base branch
- Which commit or PR caused the restructuring (use `git log --oneline --all -- <deleted-file>` to find it)
- That the work may need to be reimplemented against the new architecture

Do NOT silently close. Always explain why.

## After resolution

Run the test suite to verify the rebased code compiles and passes.
