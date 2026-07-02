# Add explicit artifact bundle/export after read-only inspection proves itself

Priority: P3 · Status: ready · Estimate: M

## Goal

Give operators a deliberate archive/export command for a run's artifacts after
the read-only artifact CLI/MCP surfaces have enough usage to justify a larger
output surface.

## Oracle

- [ ] `bb artifacts bundle <run-id> --out <path>` or an equivalent named export
      command writes a deterministic archive/manifest for a run's attempts.
- [ ] The bundle format preserves attempt numbers, relative paths, sizes,
      content types, and refusal metadata without following unsafe symlinks or
      leaking paths outside the artifact root.
- [ ] Oversized or binary artifacts are handled by explicit policy: include by
      bytes, manifest-only, or refuse with a structured reason.
- [ ] CLI help, `docs/spine.md`, and `skills/bitterblossom/` document the export
      contract only after the behavior is implemented.
- [ ] Tests cover a multi-attempt bundle, unsafe path/symlink containment, and
      deterministic manifest output.
- [ ] `./scripts/verify.sh` passes.

## Verification System

- Claim: a consuming agent or operator can collect a portable artifact bundle
  without learning the private attempt-directory layout.
- Falsifier: bundle output depends on host paths, follows symlinks outside the
  artifact root, omits attempt identity, or cannot be validated without manual
  filesystem spelunking.
- Driver: local-plane run with multiple attempts and fixture artifacts, then
  `bb artifacts bundle ...` plus manifest inspection.
- Grader: archive/manifest content matches expected relative entries and unsafe
  fixtures are refused or represented by structured policy.
- Evidence packet: command transcript, manifest sample, and gate output.

## Notes

Spawned from backlog 079. Read-only artifact inspection is now available through
CLI and MCP; bundle/export should wait for concrete usage demand so the plane
does not grow an artifact publication surface prematurely.
