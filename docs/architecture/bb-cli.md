# Removed Go Transport

The historical Go `bb` transport described here was removed in issue #703.

Its responsibilities now live in Elixir:

- sprite provisioning and repair: `Conductor.Fleet.Reconciler` + `Conductor.Sprite.create/2` + `Conductor.Sprite.provision/2`
- sprite logs: `mix conductor logs` + `Conductor.Sprite.logs/2`
- process recovery: `Conductor.Sprite.kill/1`
- dispatch: `Conductor.Sprite.dispatch/4`

Use [docs/CONDUCTOR.md](../CONDUCTOR.md) and [docs/CLI-REFERENCE.md](../CLI-REFERENCE.md) for the current operator surface.
