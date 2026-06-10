# Workspace freshness before dispatch

Priority: high
Status: abandoned
Estimate: S

## Goal
Sprites must start each dispatch with the latest origin/master checkout. Stale workspaces cause incorrect review judgments and branch confusion.

## Problem
Factory audit 5 (2026-04-03): Fern reviewed PR #837 with a workspace 2 commits behind master. It correctly observed that code didn't match "done" claims — but only because it couldn't see the auth fix (PR #836) that was already merged. This caused Fern to revert correct backlog status changes.

Additionally, Weaver started on branch `weaver/017-check-env-false-green-2` from a previous run instead of a fresh branch from origin/master.

## Sequence
- [ ] In launcher.ex launch/3, add `git fetch origin && git checkout origin/master && git reset --hard origin/master` before dispatching the agent loop
- [ ] Alternatively, add this to workspace.ex `sync_workspace/3` or bootstrap.ex
- [ ] Ensure old worktrees and branches from previous runs don't persist
- [ ] Test: verify sprite workspace is at latest master after dispatch

## Oracle
- [ ] After dispatch, sprite workspace HEAD matches origin/master HEAD
- [ ] No leftover branches from previous runs in workspace
- [ ] `mix test` passes
