# Issue 780 Plan

- [x] Replace per-run `Conductor.Retro` wiring with `Conductor.Muse`.
- [x] Add failing tests for Muse observation dispatch, reflection journaling contract, and synthesis action limits/dedup.
- [x] Implement Muse GenServer with queued observation, daily synthesis scheduling, status, and backoff/health behavior.
- [x] Wire Muse into fleet startup and health monitoring as the `bb-muse` triage sprite.
- [x] Update prompts/config/fleet declaration so Muse has an explicit persona and no code-authority prompt.
- [x] Run targeted verification, format, inspect diff scope, then open a PR.

## Review

- Muse now observes merged runs, writes local reflection journals, and performs bounded synthesis with duplicate-issue protection.
- Verification: `cd conductor && mix test` passed after `mix deps.get`; full suite reported 525 tests, 0 failures.
