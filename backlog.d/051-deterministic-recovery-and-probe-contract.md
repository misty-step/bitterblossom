# Make recovery and substrate probes deterministic under uncertainty

Priority: P1 | Status: ready | Estimate: L

## Goal

Turn inherited `running` rows and substrate probe uncertainty into bounded,
auditable operator states instead of indefinite ambiguity.

## Oracle

- [ ] Local and sprite probe contracts include malformed-marker handling,
      heartbeat or generation metadata, and explicit unknown-state outcomes.
- [ ] `awaiting_recovery` has an escalation or stale-age policy that avoids
      silent indefinite uncertainty.
- [ ] Recovery tests cover malformed pidfiles, missing markers, probe command
      failure, stale unknowns, host lease retention, and operator resolution.
- [ ] `bb recover` and `bb runs show --json` expose enough evidence for an
      operator or agent to choose resolve/replay/leave-blocked safely.
- [ ] `./scripts/verify.sh` passes.

## Children

1. Specify the probe-result state machine for local and sprite substrates.
2. Add fixture tests for malformed and stale probe artifacts.
3. Add stale `awaiting_recovery` visibility and escalation rules.
4. Update the skill/operator recipes for recovery decisions.

## Notes

Why: the runtime lane found that `awaiting_recovery` is safe but can become
sticky when probes are uncertain. The premise lane flagged side-effect recovery
as a place where the custom spine must earn trust.

Evidence:

- `docs/spine.md:339-346` says at/after-execute failures need operator
  resolution and boot recovery probes the host.
- `tests/recovery.rs` covers some inherited states, but runtime review found
  unknown/probe-failing recovery and malformed marker cases need stronger
  contract tests.
- `src/substrate/local.rs` and `src/substrate/sprites.rs` own adapter-specific
  probe assumptions.
