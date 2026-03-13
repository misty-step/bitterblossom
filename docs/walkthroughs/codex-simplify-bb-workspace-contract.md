# Walkthrough: Simplify bb Workspace Contract

## Title

Centralize the sprite workspace and completion-signal contract behind one Go module.

## Why Now

Before this branch, the thin `bb` transport still leaked one low-level protocol across several commands: workspace root paths, prompt/log filenames, and completion signal names were re-encoded in `setup`, `dispatch`, `status`, `logs`, and workspace discovery. That made a small contract change require touching multiple files that should not need to know those details.

## Before

- `dispatch.go` owned signal cleanup plus task-complete checks.
- `status.go` hard-coded the signal list for operator output.
- `logs.go` rebuilt the Ralph log path at the call site.
- `workspace_metadata.go` hard-coded prompt/log discovery names and the workspace root.
- `setup.go` repeated the same sprite path layout while provisioning the workspace.

The behavior was correct, but the module boundary was shallow: callers needed protocol details instead of asking one module for them.

## What Changed

- Added [`cmd/bb/workspace_contract.go`](../../cmd/bb/workspace_contract.go) as the Go-side source of truth for:
  - sprite workspace roots
  - Ralph/persona/prompt paths
  - completion/blocking signal filenames
  - shared shell snippets for cleanup, status display, and completion checks
- Updated `setup`, `dispatch`, `status`, `logs`, and workspace discovery to consume those helpers instead of carrying their own copies.
- Updated [`docs/COMPLETION-PROTOCOL.md`](../COMPLETION-PROTOCOL.md) so the protocol docs point at the new source of truth.

## After

Observable improvements:

- one file now owns the Go transport's workspace contract
- command code talks in terms of intent (`cleanSignalsScriptFor`, `workspaceRalphLogPath`) instead of raw filenames
- the refactor deleted more lines than it added in the touched command files while preserving behavior
- the completion protocol doc now matches the code structure reviewers will actually maintain

## Verification

Primary walkthrough artifact:

- [`codex-simplify-bb-workspace-contract-terminal.txt`](./codex-simplify-bb-workspace-contract-terminal.txt)

Persistent protecting check:

- `go test ./cmd/bb`

Supporting evidence captured in the transcript:

- diff stat for the touched transport/docs files
- `rg` output showing the new helper module and its call sites

## Residual Risk

- `scripts/ralph.sh` still has to know the shell-side signal filenames, so the contract is centralized for Go callers, not fully generated across languages.
- Process-pattern constants in `dispatch.go` and `kill.go` still encode the Ralph loop path separately; that is adjacent cleanup, not part of this PR.

## Merge Case

This branch keeps the transport behavior and tests intact while making the `bb` workspace contract deeper and easier to change safely. The win is not a new feature. The win is that operators and maintainers now have one Go module to update when the sprite workspace protocol changes.
