# 092 Plane Excision Dogfood Notes

## Goal

Prove the first product/instance split slice removes production `plane/` data
from the Docker image and remote build context, while preserving the existing
repo gate and local config flow.

## Slice Shipped

- Docker image no longer runs `COPY plane ./plane`.
- `.dockerignore` excludes `plane`, so remote image builds do not upload
  instance config in the build context.
- `BB_PLANE_DIR=/app/plane` is the runtime config root.
- Fly volume `bb_plane_data` is mounted at `/app/plane`, carrying
  `plane.toml`, `agents/`, `tasks/`, and `.bb/plane.db`.
- `./scripts/verify.sh` now fails on `COPY plane` or a missing dockerignore
  exclusion.
- Deployment docs include the old-volume migration path.

## Evidence

```sh
! rg -n '^\s*COPY\s+plane(\s|$)' Dockerfile \
  && grep -qx 'plane' .dockerignore \
  && echo 'image/context plane exclusion: ok'
```

Result: `image/context plane exclusion: ok`.

```sh
./target/debug/bb --config examples/demo-plane check
BB_PLANE_DIR=examples/local-plane ./target/debug/bb check --json >/dev/null
```

Result: demo plane validated and runtime `BB_PLANE_DIR` config loading worked.

```sh
docker build -t bitterblossom-plane-excision-test .
docker run --rm --entrypoint sh bitterblossom-plane-excision-test -c '
  printf "BB_PLANE_DIR=%s\n" "$BB_PLANE_DIR"
  test "$BB_PLANE_DIR" = /app/plane
  test -d /app/plane
  test ! -e /app/plane/plane.toml
  test ! -d /app/plane/tasks
  test ! -d /app/plane/agents
  echo image_has_no_plane_config
'
```

Result: Docker build used a 1 kB context; container printed
`BB_PLANE_DIR=/app/plane` and `image_has_no_plane_config`.

```sh
./scripts/verify.sh
```

Result: all gates green, including the new product/instance split guard.

## Dogfood Notes

- The first local Docker proof caught a real portability bug: the image always
  downloaded `sprite-linux-amd64`, then tried to execute it in an arm64 local
  build. The Dockerfile now selects `sprite-linux-${TARGETARCH}` for `amd64`
  and `arm64`, so the proof works locally and still supports Fly's Linux build.
- The repo is not public-able yet because `plane/` is still tracked. This slice
  closes image/context leakage; the next slice must relocate or remove tracked
  instance data and add a scan/gate for private topology, budgets, and allowlists.
- No Fly deploy or volume migration was run in this slice. The migration is
  outward-facing and should happen deliberately after review.
