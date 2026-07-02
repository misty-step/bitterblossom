# 092 Tracked Plane Excision Dogfood Notes

## Goal

Make the product checkout public-able by removing production instance config
from Git tracking while preserving public tests, examples, docs, and local
operator runtime data.

## Slice Shipped

- `plane/` was removed from Git tracking with `git rm --cached -r plane`.
- `canary-services.toml` was removed from Git tracking with
  `git rm --cached canary-services.toml`.
- `.gitignore` now keeps local runtime `plane/` and `canary-services.toml`
  out of future commits.
- `./scripts/verify.sh` fails if `git ls-files 'plane/**'` or
  `git ls-files canary-services.toml` returns entries.
- Product lifecycle tests load `tests/fixtures/public-plane/` instead of the
  operator plane.
- Model catalog checks load `tests/fixtures/model-catalog-agents/` instead of
  production agents.
- Operations docs and `bb-dogfood` require an explicit `BB_RUNTIME_PLANE` or
  service-side `BB_PLANE_DIR`.

## Evidence

```sh
git ls-files 'plane/**' canary-services.toml
```

Result: no tracked entries.

```sh
cargo test --test lifecycle_reflex --test model_catalog --test cli_contract_docs --test skill_artifacts
```

Result: focused public-fixture and docs-contract tests passed.

```sh
scripts/check-model-catalog.sh --catalog tests/fixtures/openrouter-models-current.json --json
```

Result: `status=pass`, `configured=8`, `missing=0`, `metadata_gaps=0`,
`docs_missing=0`.

## Boundary

This PR does not deploy or migrate Fly. The local operator plane remains on
disk because the removal used `git rm --cached`; it is instance data and is now
ignored by the product repo.
