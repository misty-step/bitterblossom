# Epic: resume releasable BB artifacts

Priority: P2 | Status: ready | Estimate: M

## Goal

Resume regular Bitterblossom releases so downstream consumers can pin versions
instead of tracking a moving source checkout.

## Oracle

- [ ] Release workflow produces versioned artifacts or documented install
      commands for the `bb` binary.
- [ ] Landmark release intelligence runs on published releases and writes
      repo-owned release notes.
- [ ] Versioning policy is documented: what changes bump major/minor/patch and
      what compatibility promises apply to CLI/API/MCP JSON.
- [ ] A dry-run or fixture release proves artifacts, notes, checksums, and
      install docs before the first resumed public release.
- [ ] Consumers can pin a release in docs or examples.
- [ ] `./scripts/verify.sh` passes.

## Children

- [ ] Audit existing release workflow and last release state.
- [ ] Define versioning and compatibility policy.
- [ ] Build/package binary artifacts and checksums.
- [ ] Landmark release-notes proof.
- [ ] Consumer pinning docs.

## Notes

Landmark integration already exists in this repo. This epic is about making
release output routine again, not adding release prose by hand.
