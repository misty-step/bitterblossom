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
- `primitives/skills/skills-index.yaml` is reference-only in P0. Do not migrate
  skill bodies into this repo until P3.
- `roster sync` is a P2 stub and bb materialization is a P1 stub.
- No secret values in declarations or registries; env refs only.

## Gate

Run before claiming repo changes are complete:

```sh
cargo fmt --all -- --check
cargo clippy --workspace --all-targets -- -D warnings
cargo test --workspace
```
