# Release policy

Backlog 097 (epic: resume releasable BB artifacts) — first slice. This
document is the versioning policy the epic's acceptance criteria require;
the mechanism it describes is `scripts/bb-cut-release`.

## Versioning

`bb` follows semver (`MAJOR.MINOR.PATCH`) against the surface an operator or
agent actually depends on: CLI flags/subcommands, `plane.toml`/`agents/*.toml`/
`task.toml` config fields, the `/api/*` read routes, and the ledger schema a
backup/restore depends on (`docs/operations/README.md`).

- **Patch** — internal fixes, dependency bumps, doc/example changes, or a bug
  fix that does not change any documented CLI flag, config field, API route
  shape, or ledger schema version. Safe to take without reading anything.
- **Minor** — additive, backward-compatible surface: a new CLI subcommand or
  flag, a new optional config field, a new `/api/*` route, a new gate
  mechanism (e.g. `bb submit waive`, backlog 088). Existing configs, scripts,
  and API callers keep working unchanged.
- **Major** — anything a consumer must change for: a removed/renamed CLI
  flag or config field, a changed `/api/*` response shape, a ledger schema
  version bump (`LEDGER_SCHEMA_VERSION` in `src/ledger.rs`) that isn't purely
  additive, or a changed default that alters existing behavior.

When in doubt, treat it as the more disruptive bump — a false "minor" that
breaks a consumer is worse than an unnecessary "major".

## Cutting a release

```bash
scripts/bb-cut-release --dry-run          # inspect: current/next version, tag, commit count
scripts/bb-cut-release --bump minor --live  # tag + push + `gh release create --generate-notes`
```

`--dry-run` is the default; `--live` is required to actually tag, push, and
create the GitHub Release. The script refuses to run live with an unclean
working tree, off `master`, or against a tag that already exists — see
`scripts/bb-cut-release --help` for the full precondition list.

Publishing the release automatically triggers `.github/workflows/
landmark-release.yml` (`on: release: published`), which synthesizes
human-readable release notes via Landmark and writes them to
`docs/releases/<version>.md` and `docs/releases/releases.json` — that piece
was already wired before this backlog item; this slice supplies the part
that was missing: an actual, repeatable way to cut the release that
triggers it.

## Pinning a release

Once a release exists, a consumer can pin it instead of tracking a moving
`master`:

```bash
cargo install --git https://github.com/misty-step/bitterblossom --tag v<version> --locked
```

or, for a vendored/cloned checkout:

```bash
git clone --branch v<version> --depth 1 https://github.com/misty-step/bitterblossom.git
```

## Explicitly out of scope for this slice

- **Prebuilt binary artifacts and checksums.** This slice cuts a source-tagged
  GitHub Release only; `cargo install --git --tag` (or a source checkout) is
  the documented install path. Cross-compiled binaries + checksums are
  tracked as remaining epic scope (backlog 097), not solved here.
- **Automatic/scheduled release cutting** (e.g. release-please-style,
  triggered on every merge to `master`). This slice is a deliberate,
  human-invoked `--live` action — automation on top of it is remaining epic
  scope.
- **The actual first version number.** `Cargo.toml` still carries the
  placeholder `0.1.0` from `cargo init`; this slice does not choose or bump
  the first real v3 version number — that is an operator decision, not
  something to default silently. `scripts/bb-cut-release` computes the next
  version from whatever `Cargo.toml` says at invocation time, whenever that
  first decision is made.
