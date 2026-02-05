---
description: Tidy workspace, create semantically meaningful commits, and push
---

# COMMIT

Analyze changes, tidy up, create semantically meaningful commits, and push.

## 1. Analyze

```bash
git status --short
git diff --stat HEAD
```

Categorize each changed/untracked file:
- **Commit**: Quality changes that belong in the repo
- **Gitignore**: Generated/temporary files that should be ignored
- **Delete**: Cruft, experiments, or files no longer needed

## 2. Tidy

- Add patterns to `.gitignore` as needed
- Remove files that shouldn't exist
- Working directory should only contain intentional changes

## 3. Group Commits

| Change Type | Prefix |
|-------------|--------|
| New feature | `feat:` |
| Bug fix | `fix:` |
| Documentation | `docs:` |
| Refactoring | `refactor:` |
| Performance | `perf:` |
| Tests | `test:` |
| Build/deps | `build:` |
| CI/CD | `ci:` |
| Maintenance | `chore:` |

One logical change per commit. Each independently meaningful.

## 4. Create Commits

```bash
git add <relevant files>
git commit -m "<type>(<scope>): <description>

<body if needed>

Co-Authored-By: <sprite-name> <noreply@anthropic.com>"
```

Subject: imperative mood, lowercase, no period, <=50 chars.
Body: explain *why*, not *what*.

## 5. Quality Check

```bash
# Run whatever quality gates the repo has
pnpm lint 2>/dev/null || npm run lint 2>/dev/null || true
pnpm typecheck 2>/dev/null || true
pnpm test 2>/dev/null || npm test 2>/dev/null || true
```

Fix issues before proceeding.

## 6. Push

```bash
git fetch origin
git pull --rebase origin $(git branch --show-current)
git push origin HEAD
```

## Safety

- Never force push
- Never push to main/master directly
- Always fetch before push
- If unsure about a deletion, ask first
