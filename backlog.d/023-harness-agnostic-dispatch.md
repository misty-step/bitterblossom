# Harness-agnostic dispatch

Priority: high
Status: ready
Estimate: M

## Goal
Support Claude Code as a first-class harness alongside Codex. Support arbitrary harnesses via a thin interface.

## Current state
The conductor has `@harness_modules` in launcher.ex with `"claude-code" => Conductor.ClaudeCode` and `@default_harness Conductor.Codex`. The fleet.toml `harness` field selects between them. The basic abstraction exists but Claude Code support may not be fully tested for the autonomous loop path.

## Sequence
- [ ] Verify Claude Code harness works for autonomous sprite loops (not just one-shot)
- [ ] Test the full loop: launch → bootstrap → dispatch → monitor → recover with Claude Code harness
- [ ] Add `sprites-ex` SDK as a transport option (backlog.d/013)
- [ ] Document the harness interface contract: what a harness module must implement
- [ ] Test with a heterogeneous fleet: some sprites on Codex, some on Claude Code

## Oracle
- [ ] A fleet with mixed harnesses (Codex + Claude Code) runs and self-heals
- [ ] Adding a new harness requires implementing one module, not changing the conductor
