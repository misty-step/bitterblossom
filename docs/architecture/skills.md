# Skills Architecture

Managed sprites receive the repo-local `base/skills/` tree during fleet reconciliation.

## Source Of Truth

- local files: `base/skills/`
- upload path on sprites: `/home/sprite/.claude/skills/`
- provisioning implementation: `Conductor.Sprite.provision/2`

## Operational Notes

- skills are version-pinned to the repo revision that provisioned the sprite
- re-run `mix conductor fleet --reconcile` after changing `base/skills/`
- there is no separate `bb setup` step anymore
