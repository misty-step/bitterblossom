# Harness-agnostic dispatch

Priority: high
Status: ready
Estimate: M

## Goal
Support Claude Code as a first-class harness alongside Codex. Verify the harness abstraction works for heterogeneous fleets and document the interface contract.

## Current state
- `Conductor.Codex` — implements Codex dispatch (primary, well-tested)
- `Conductor.ClaudeCode` — implements Claude Code dispatch (exists, not tested for autonomous loops)
- `Launcher.launch/3` — selects harness via `@harness_modules` map, defaults to Codex
- fleet.toml `harness` field — selects `"codex"` or `"claude-code"` per sprite

## Sequence
- [ ] Read `Conductor.ClaudeCode` module: verify it implements `dispatch_command/1` correctly for autonomous loop use
- [ ] Test Claude Code harness end-to-end: create a test sprite with `harness = "claude-code"`, launch, verify loop starts and runs
- [ ] Verify spellbook bootstrap works for Claude Code harness (skill symlinks go to `.claude/skills/` not `.codex/skills/`)
- [ ] Document the harness interface contract: a harness module must implement `dispatch_command(opts) :: [String.t()]` returning the shell command parts
- [ ] Test with heterogeneous fleet: fleet.toml declares one Codex sprite and one Claude Code sprite, both launch and self-heal independently
- [ ] Add `"claude"` as an alias for `"claude-code"` in `@harness_modules` for brevity

## Oracle
- [ ] A fleet with mixed harnesses (Codex + Claude Code) can launch, run, and self-heal
- [ ] Adding a new harness requires implementing one module with `dispatch_command/1`, nothing else
- [ ] The harness interface is documented in a module doc or reference file
