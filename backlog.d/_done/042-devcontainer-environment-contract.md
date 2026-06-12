# Honor devcontainer.json as the environment contract on sprites

Priority: P3 · Status: abandoned · Estimate: M

## Goal

A repo with a devcontainer.json runs workloads on a sprite with zero
bespoke provisioning lore — the standard file, not checkpoint folklore,
declares what the workspace needs.

## Oracle

- [ ] A fresh repo with a devcontainer.json (declaring at minimum a
      postCreateCommand and tool dependencies) completes a real workload
      run on a sprite that has never been hand-provisioned for it
- [x] Conformance scope is documented honestly in docs/spine.md: the
      plane does not honor devcontainer.json in v1; setup remains task
      `pre_command` plus sprite checkpoints
- [x] Existing checkpoint-based provisioning still works; no substrate
      rewrite was attempted

## Notes

**Why:** from the Ona research (2026-06-11) — Ona environments are
devcontainer-defined, which makes any repo "agent-ready" by an industry
standard instead of by provisioning lore (our sprite checkpoints are
bespoke knowledge that lives in harness-kit references). It is also the
substrate-neutral environment contract: checkpoints are a sprites-only
concept, while devcontainer.json travels with the repo to any future
substrate (Cloudflare containers would honor it natively — see 044).

## Verdict (2026-06-12)

Abandoned. The premise challenge resolved against implementing a partial
devcontainer shim in the Rust spine.

The official Dev Container metadata reference defines `features` as
container Feature IDs/options and lifecycle commands such as
`postCreateCommand` as commands that run after the dev container has been
created. The Features reference defines each Feature as a packaged unit
with `devcontainer-feature.json` plus `install.sh`, with installation
order, options, user/container environment, and lifecycle-hook behavior.

Sprites are restored Fly machines, not container builds. Implementing only
`postCreateCommand` would duplicate the existing task `pre_command` while
ignoring image/build, Feature installation, mounts, users, container env,
and Docker Compose semantics. Implementing real Feature support would
move package-manager and container-runtime judgment into the ≤5k LOC
spine, directly against ADR-005's "no workload logic in the plane" and
the current `src LOC: 4999` budget.

The honest contract now lives in `docs/spine.md` under "Environment
Contract": use task `pre_command` for per-run repo setup, sprite
checkpoints for machine-level tools, and revisit devcontainer support
only behind a substrate that can delegate to a real devcontainer
implementation/container runtime.

## Evidence

- Live repo: `src/substrate/sprites.rs` restore/checkpoint flow is
  restore -> repo sync -> card/event/report upload -> `pre_command` ->
  harness execute; `src/substrate/local.rs` mirrors that for local
  workspaces.
- Official spec lookup:
  `https://containers.dev/implementors/json_reference/` documents
  `features`, image/build settings, and lifecycle command semantics.
  `https://containers.dev/implementors/features/` documents Features as
  installable units with metadata plus `install.sh`.
- `docs/spine.md` now records the unsupported boundary and the supported
  alternatives.
