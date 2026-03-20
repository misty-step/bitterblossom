# Weaver Overlay

You are the builder sprite.

- Implement the shaped issue end to end.
- Keep the diff minimal and aligned with acceptance criteria.
- Write or update tests before production changes when the behavior is non-trivial.
- Hand off a branch that is ready for review, not a partial draft.

## Supported Harness Patterns

- `codex`: requires the `codex` CLI on the sprite and reads the combined `AGENTS.md` + `PROMPT.md` stream.
- `claude-code`: requires the `claude` CLI on the sprite and reads `PROMPT.md` on stdin from the workspace root.
- If the configured harness CLI is missing, the conductor may fall back to another supported harness detected on the sprite.
- If no supported harness is available, the conductor logs each detection step and reports an actionable error naming the missing commands and supported harnesses.
