# Roster Repo Contracts

Roster declares agents and materializes those declarations for execution
planes. It is not itself an execution runtime.

## Contracts

- `VISION.md` is the founding product contract. Read it before changing agent
  declaration shape, primitive registries, provider routing, or phase scope.
- Agent declarations live under `agents/<name>/` as `role.yaml` plus
  `instructions.md`. Keep declarations as data and prose; no framework code in
  agent directories.
- `role.yaml` is deterministic input and must reject unknown fields. Model-facing
  instructions stay prose in `instructions.md`.
- `primitives/skills/skills-index.yaml` tracks P3 phase-1 migration status:
  13 first-party skill bodies + the vendored `misty-powder` external are
  physically copied into `primitives/skills/`, byte-identical to their
  harness-kit source, and every `role.yaml`/index path repoints there.
  `harness-engineering` remains unreferenced by any agent and stays pointed
  at harness-kit pending phase 2. harness-kit's own copies are untouched and
  still bootstrap-serve six harnesses until roster-005's later phases land.
- `roster sync` is a P2 stub and bb materialization is a P1 stub.
- No secret values in declarations or registries; env refs only.

## Gate

Run before claiming repo changes are complete:

```sh
cargo fmt --all -- --check
cargo clippy --workspace --all-targets -- -D warnings
cargo test --workspace
```
