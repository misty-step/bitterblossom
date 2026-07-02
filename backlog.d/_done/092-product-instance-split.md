# Epic: product/instance split and public-able repo

Priority: P0 | Status: done | Estimate: XL

## Goal

Separate Bitterblossom the product from the Misty Step production instance.
Tracked product code must be public-able; instance task cards, org allowlists,
sprite hosts, budgets, and repo-specific secrets belong outside the product image.

## Oracle

- [x] The Docker image no longer `COPY`s the production `plane/` directory.
- [x] Production config is loaded from an instance source: private repo, Fly
      volume, mounted secret bundle, or explicit `bb config pull` path.
- [x] `examples/demo-plane` remains the public reference plane and validates in
      the repo gate.
- [x] Runtime config can be reloaded or redeployed without rebuilding the product
      binary/image for ordinary task-card and budget changes.
- [x] A tracked-file scan finds no private topology, tailnet names, personal
      paths, real repo allowlists, or instance data in product-owned files.
- [x] Production deploy docs explain how an operator supplies an instance plane.
- [x] `./scripts/verify.sh` passes.

## Children

- [x] Decide instance-config source and migration path for current `plane/`.
- [x] Dockerfile and Fly launch changes to mount/pull runtime config.
- [x] Public-able scan/gate for tracked files.
- [x] Demo/reference plane cleanup so clone-onboarding still works.
- [x] Config reload or low-risk redeploy path for budget/filter changes.
- [x] Delete or relocate stale instance artifacts such as `canary-services.toml`
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

2026-07-02 slice: removed production `plane/` and stale
`canary-services.toml` from Git tracking while leaving local ignored files on
disk for the operator. The repo now ignores `/plane/` and `/canary-services.toml`;
`./scripts/verify.sh` fails if either path is tracked again. Product tests use
`tests/fixtures/public-plane/`, model-catalog checks use
`tests/fixtures/model-catalog-agents/`, and the gate validates
`examples/demo-plane`, `examples/local-plane`, and the public fixture instead
of the private runtime plane. Operations docs and the dogfood skill now require
an explicit `BB_RUNTIME_PLANE` or Fly `BB_PLANE_DIR`.

2026-07-02 final closeout:

- PR #887 loaded the production plane from a runtime volume instead of the
  product image.
- PR #888 excised tracked production plane data and stale instance config from
  Git while leaving local ignored runtime files on disk.
- Verification: `./scripts/verify.sh` passed during backlog 100 closeout.
