# Plan: Centralize default-branch detection (#432)

## Problem

`dispatch.go` hard-codes `master || main` in two independent places:
1. Sync script — `git checkout master 2>/dev/null || git checkout main`
2. Verify script — `git log origin/master..HEAD || git log origin/main..HEAD`

If the default branch is neither `master` nor `main`, both paths fail independently.

## Solution

Detect once (`origin/HEAD`), thread value through both paths.

## Steps

- [x] Read codebase, understand duplication
- [ ] Add `detectDefaultBranchScript` const and `detectDefaultBranchWithRunner` func to `dispatch_checks.go`
- [ ] Update `verifyWorkScriptFor` in `dispatch.go` to accept `defaultBranch string` param
- [ ] In `runDispatch`, detect default branch after workspace is resolved; thread into sync script and `verifyWorkScriptFor`
- [ ] Add regression tests for `detectDefaultBranchWithRunner` in `dispatch_test.go`
- [ ] Update `TestVerifyWorkScriptForQuotesWorkspace` for new signature
- [ ] `go test ./cmd/bb/...` — all pass

## Constraints

- No new packages, no new Go files (all in `cmd/bb/`)
- Narrow diff — do not touch anything outside dispatch transport
- Fallback to `"main"` when `origin/HEAD` is unset (sane default)
