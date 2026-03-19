# Issue 719 Walkthrough

## Goal

Make Thorn act like a quality guardian instead of a CI appeasement bot by staging shared plus Thorn-specific persona files into the live fixer workspace before dispatch.

## Before

- Thorn only received a thin fixer prompt.
- The live dispatch workspace had no Thorn-specific `CLAUDE.md`, `AGENTS.md`, or skill pack.
- The prompt told Thorn to focus on making CI green.

## After

- `Conductor.Persona` builds a validated manifest for shared plus Thorn overlays.
- `Conductor.Sprite.dispatch/4` stages those assets into the target workspace before the agent starts.
- Thorn's prompt now requires context gathering, CI diagnosis, fix planning, and invariant verification before code changes.

## Verification

Run from [`conductor/`](../../conductor):

```bash
mix test test/conductor/sprite_dispatch_test.exs test/conductor/prompt_test.exs test/conductor/fixer_test.exs
mix test
```

## Persistent Checks

- [`test/conductor/sprite_dispatch_test.exs`](../../conductor/test/conductor/sprite_dispatch_test.exs) proves the Thorn overlay is copied into the workspace before execution.
- [`test/conductor/prompt_test.exs`](../../conductor/test/conductor/prompt_test.exs) proves the fixer prompt routes through the new skill workflow and fallback instructions.
- [`test/conductor/fixer_test.exs`](../../conductor/test/conductor/fixer_test.exs) proves fixer dispatch still works with the new role overlay option.

## Residual Risk

The live manual sprite dispatch from the acceptance criteria was not run in this workspace, so the remaining risk is harness-specific behavior on a real remote sprite rather than local conductor logic.
