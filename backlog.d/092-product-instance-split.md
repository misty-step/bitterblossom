# Epic: product/instance split and public-able repo

Priority: P0 | Status: ready | Estimate: XL

## Goal

Separate Bitterblossom the product from the Misty Step production instance.
Tracked product code must be public-able; instance task cards, org allowlists,
sprite hosts, budgets, and repo-specific secrets belong outside the product image.

## Oracle

- [x] The Docker image no longer `COPY`s the production `plane/` directory.
- [x] Production config is loaded from an instance source: private repo, Fly
      volume, mounted secret bundle, or explicit `bb config pull` path.
- [ ] `examples/demo-plane` remains the public reference plane and validates in
      the repo gate.
- [ ] Runtime config can be reloaded or redeployed without rebuilding the product
      binary/image for ordinary task-card and budget changes.
- [ ] A tracked-file scan finds no private topology, tailnet names, personal
      paths, real repo allowlists, or instance data in product-owned files.
- [ ] Production deploy docs explain how an operator supplies an instance plane.
- [ ] `./scripts/verify.sh` passes.

## Children

- [ ] Decide instance-config source and migration path for current `plane/`.
- [x] Dockerfile and Fly launch changes to mount/pull runtime config.
- [ ] Public-able scan/gate for tracked files.
- [ ] Demo/reference plane cleanup so clone-onboarding still works.
- [ ] Config reload or low-risk redeploy path for budget/filter changes.
- [ ] Delete or relocate stale instance artifacts such as `canary-services.toml`
      after proving they are not product inputs.

## Notes

The groom report found product/instance fusion: the product image bakes in the
Misty Step plane. This epic is prerequisite to making the repo safely public and
to letting other operators adopt the same shape without inheriting our instance.

2026-07-02 slice: the product image no longer copies `plane/`, `.dockerignore`
excludes `plane/` from remote build context, `fly.toml` mounts
`bb_plane_data` at `/app/plane`, and `BB_PLANE_DIR=/app/plane` makes
`plane.toml`, `agents/`, `tasks/`, and `.bb/plane.db` runtime volume data.
`./scripts/verify.sh` now fails if `COPY plane` or a missing dockerignore
exclusion reintroduces image/context leakage. Docker proof built the image with
a 1 kB context and confirmed `/app/plane` is empty of `plane.toml`, `tasks/`,
and `agents/` until a runtime volume supplies them. The repo still tracks
`plane/`; public-able tracked-file excision remains the next child.
