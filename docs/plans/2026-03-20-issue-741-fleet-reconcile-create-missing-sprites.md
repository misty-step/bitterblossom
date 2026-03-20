Issue: #741

Problem
- `fleet.toml` declares `bb-weaver`, `bb-thorn`, `bb-fern`, and `bb-muse`, but the real org still has `bb-builder`, `bb-fixer`, and `bb-polisher`.
- `mix conductor fleet --reconcile` is documented as the supported repair path, yet the reconciler only provisions reachable sprites and leaves missing declared sprites degraded forever.

Acceptance Criteria
- Fleet reconciliation converges declared sprite names when the backing sprite does not yet exist.
- Reconciler tests cover the create-then-provision path and the failure path when creation fails.
- Operator docs describe that `--reconcile` creates missing declared sprites before provisioning them.

Implementation Slice
- Add a sprite creation primitive in `Conductor.Sprite`.
- Teach `Conductor.Fleet.Reconciler` to create missing sprites when health checks fail because the sprite is absent.
- Preserve the degraded path for transient transport failures so reconciliation does not create duplicate sprites.
- Add focused unit tests around the new branch and update operator docs.

Open Risks
- Old legacy sprites will still exist until an operator deletes or renames them; this lane only ensures the declared fleet becomes bootable.
- Real end-to-end provisioning still depends on local org/auth credentials and is only partially verifiable in unit tests here.
