# Completion Protocol

The conductor uses file-based completion signals inside the sprite worktree.

## Canonical Signals

- `TASK_COMPLETE`
- `TASK_COMPLETE.md` (legacy compatibility)
- `BLOCKED.md`

## Current Implementation

- prompt construction: `conductor/lib/conductor/prompt.ex`
- run-state artifact checks: `conductor/lib/conductor/run_server.ex`
- sprite dispatch and log teeing: `conductor/lib/conductor/sprite.ex`
- worktree lifecycle: `conductor/lib/conductor/workspace.ex`

## Notes

- Dispatch writes agent output to `ralph.log` inside the worktree.
- `mix conductor logs <sprite>` tails that file.
- The deleted Go `bb` transport is no longer part of the completion path.
