# Honor devcontainer.json as the environment contract on sprites

Priority: P3 · Status: pending · Estimate: M

## Goal

A repo with a devcontainer.json runs workloads on a sprite with zero
bespoke provisioning lore — the standard file, not checkpoint folklore,
declares what the workspace needs.

## Oracle

- [ ] A fresh repo with a devcontainer.json (declaring at minimum a
      postCreateCommand and tool dependencies) completes a real workload
      run on a sprite that has never been hand-provisioned for it
- [ ] Conformance scope is documented honestly in docs/spine.md: which
      devcontainer fields the sprites substrate honors (likely
      postCreateCommand + features-as-install-hints), which it ignores
      (full container image semantics), and why
- [ ] Existing checkpoint-based provisioning still works; devcontainer
      support is additive in the prepare phase, not a rewrite

## Notes

**Why:** from the Ona research (2026-06-11) — Ona environments are
devcontainer-defined, which makes any repo "agent-ready" by an industry
standard instead of by provisioning lore (our sprite checkpoints are
bespoke knowledge that lives in harness-kit references). Premise
challenge to resolve at shape time: sprites are Fly machines, not
container hosts — full devcontainer semantics (image, features) may
require docker-in-VM and is likely not worth it; partial conformance
(postCreateCommand + declared tools) may capture most of the value.
If shaping concludes even that is thin value for one operator's repos,
abandon with the verdict recorded — the checkpoint flow already works.
