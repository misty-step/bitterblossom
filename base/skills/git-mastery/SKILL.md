---
name: git-mastery
user-invocable: false
description: "Git workflow enforcement and best practices: atomic commits, conventional format, branch strategy, merge decisions."
allowed-tools:
  - Read
  - Grep
  - Glob
  - Bash
---

# Git Mastery

Distributed-first, async-friendly git workflows with automated quality gates.

## Commits: Atomic + Conventional

**Every commit must be:**
- Single logical change (use `git add -p` for selective staging)
- Complete (code + tests + docs together)
- Independently buildable and testable
- Describable without "and" in subject

**Format:** `type(scope): subject` (50 chars max)
```
feat(auth): add OAuth2 login flow
fix(api): handle null response in user endpoint
refactor(db): extract query builder
```
Types: `feat`, `fix`, `docs`, `style`, `refactor`, `perf`, `test`, `build`, `ci`, `chore`

BREAKING CHANGE: Add footer `BREAKING CHANGE: description` → triggers MAJOR version.

## Branches: Short-Lived + Typed

**Naming:** `type/issue-description` (lowercase, hyphens, <60 chars)
```
feature/123-oauth-login
fix/456-null-pointer-api
hotfix/critical-auth-bypass
```

**Rules:**
- Max 3 days old (escalate if longer)
- Delete immediately after merge
- Rebase onto main daily
- Never push directly to main

## Merge Strategy

| Condition | Strategy |
|-----------|----------|
| <3 days, single author, atomic | Rebase (linear history) |
| Multi-author or external PR | Merge (preserve context) |
| Many fixup/experimental commits | Squash (clean history) |

## PR Workflow

1. CI passes before human review (lint, type-check, test, security)
2. CODEOWNERS auto-assigns reviewers
3. 1-2 approvals required
4. Squash fixup commits before merge
5. Branch auto-deleted on merge

Context-rich PRs: motivation, alternatives considered, areas of concern.

## Anti-Patterns

- Mixed commits ("fix auth and update logging")
- Long-lived branches (>1 week without escalation)
- WIP/fix typo commits in main history
- Manual merge strategy choice (use decision tree)
- Large binary files in git history
