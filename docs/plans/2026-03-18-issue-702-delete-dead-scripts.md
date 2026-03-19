# Issue 702 Dead Scripts Removal Plan

> Scope issue: [#702](https://github.com/misty-step/bitterblossom/issues/702)

## Goal

Delete the dead shell entrypoints that were superseded by the Elixir conductor so the repo's live surface matches the current architecture.

## Product Spec

### Problem

`scripts/` still advertises shell entrypoints that no longer define Bitterblossom's runtime path. They duplicate or contradict the conductor and `bb`, which makes operators and agents load dead workflows and preserves file-path coupling that the current architecture is trying to delete.

### Intent Contract

- Intent: remove obsolete shell scripts and the obsolete `ralph-prompt-template.md` symlink so the repository only exposes supported runtime entrypoints.
- Success Conditions: the 13 dead scripts are deleted, the explicitly retained files remain, and live code/docs/tests no longer point at the removed files as if they are supported.
- Hard Boundaries: keep `scripts/onboard.sh`, `scripts/lib.sh`, `scripts/test_runtime_contract.py`, `scripts/builder-prompt-template.md`, and `scripts/glance.md`; do not rewrite conductor behavior in this lane.
- Non-Goals: remove historical archive references, delete all `ralph.sh`-era documentation in one sweep, or redesign the remaining `cmd/bb` runtime in the same lane.

## Technical Design

### Approach

1. Delete the dead shell scripts and the `scripts/ralph-prompt-template.md` symlink.
2. Update repo-owned live surfaces that still treat those files as current implementation details.
3. Preserve historical and archival documents when they are clearly marked as old context rather than live operator guidance.
4. Verify the cut with targeted searches plus the tests that still exercise the remaining workspace/runtime contract.

### Files to Modify

- `scripts/` — delete obsolete scripts and the obsolete symlink; keep the files named in the issue.
- `README.md`, `CLAUDE.md`, `glance.md`, `scripts/glance.md`, selected docs under `docs/` — remove or reframe live references to deleted scripts.
- `cmd/bb/*.go`, `cmd/bb/*_test.go`, `base/hooks/test_workspace_contract.py` — remove hardcoded references to deleted shell assets that are no longer part of the supported path.

### Implementation Sequence

1. Confirm the live reference surface and separate active contracts from archive/history.
2. Remove the dead scripts and obsolete symlink in one slice.
3. Update live code/tests/docs to point at the supported Elixir conductor and retained prompt/template files.
4. Re-run searches and targeted tests until no live references remain.

### Risks & Mitigations

- Risk: a `cmd/bb` path still assumes `scripts/ralph.sh` or related assets exist.
  Mitigation: inspect `cmd/bb` before deletion and either retarget or trim the related behavior in the same slice.
- Risk: docs cleanup overreaches into archival material and destroys useful history.
  Mitigation: keep clearly archived docs intact unless they claim the deleted files are still current.
- Risk: deleting files before updating tests hides the real breakage.
  Mitigation: use the remaining workspace/runtime tests as the first verification pass after the cut.

## Implementation Notes

- Current `scripts/` inventory on this branch was already reduced to `builder-prompt-template.md`, `lib.sh`, `onboard.sh`, `sentry-watcher.sh`, and `test_runtime_contract.py`; the cut-list shell entrypoints and `ralph-prompt-template.md` symlink were already absent.
- The implemented slice therefore focused on two residual gaps:
  - add a runtime-contract regression test that keeps the removed shell entrypoints absent and prevents supported surfaces from naming them again
  - update supported docs and architecture notes so the repo no longer advertises the retired shell layer as current runtime surface
- Archive/history material remains intentionally untouched unless it claims a removed shell asset is still part of the supported runtime path.
