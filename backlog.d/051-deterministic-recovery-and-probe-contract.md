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
2. Add fixture tests for malformed and stale probe artifacts. (malformed local
   and sprite pidfile coverage started 2026-06-14; stale recovery visibility
   covered in `bb status --json`)
3. Add stale `awaiting_recovery` visibility and escalation rules. (started:
   one-hour status escalation action)
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

## Delivery Notes

### 2026-06-14 malformed probe evidence slice

- Fixed `sprites` probing so a malformed remote `/tmp/<marker>.pid` returns
  `Unknown` rather than `Dead`; this prevents recovery from releasing a host
  lease when the probe evidence is corrupt.
- Added recovery coverage proving a malformed local pidfile moves the run to
  `awaiting_recovery`, preserves the host lease, and exposes the `boot_probe`
  event through `bb runs show --json`.
- Updated the operator recipe and spine contract so agents know that unknown
  probes require side-effect inspection and that missing/malformed pidfiles are
  unknown, not dead.
- Remaining 051 scope: stale-age policy/escalation, full probe state-machine
  spec, and any additional malformed/missing/stale fixture coverage.

### 2026-06-14 stale recovery status slice

- Added `bb status --json` stale recovery visibility: an `awaiting_recovery`
  run keeps the normal `resolve_after_side_effect_inspection` action while
  fresh, then becomes `escalate_stale_recovery` after one hour.
- The action includes `age_seconds` and `stale_after_seconds` so agents can
  tell fresh uncertainty from operator-stale uncertainty without parsing prose.
- This is deliberately not automatic replay or resolution; side-effecting runs
  still require operator inspection and `bb runs resolve`.
- Remaining 051 scope: full probe state-machine spec plus broader
  missing/probe-command-failure/stale fixture coverage.
