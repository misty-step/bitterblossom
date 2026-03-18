# Issue 702 Workpad

## Problem

`scripts/` still exposes dead shell entrypoints from the pre-conductor runtime. Some live code and docs still point at those files as if they are part of the supported path, especially around `bb` setup/dispatch and context docs.

## Acceptance Criteria

- Delete the 13 obsolete scripts listed in issue `#702`.
- Delete the obsolete `scripts/ralph-prompt-template.md` symlink.
- Keep `scripts/onboard.sh`, `scripts/lib.sh`, `scripts/test_runtime_contract.py`, `scripts/builder-prompt-template.md`, and `scripts/glance.md` if present.
- Remove live code and doc references that still treat deleted scripts as supported.

## Implementation Slice

1. Retarget `cmd/bb` away from repo-owned shell assets so setup/dispatch no longer require deleted files.
2. Update contract tests and live docs to describe the supported conductor + `bb` path.
3. Delete the obsolete scripts and rerun targeted searches/tests.

## Risks

- `cmd/bb` still assumes a repo-owned remote loop script exists; deleting the files without changing transport code would break setup/dispatch.
- Some docs under `docs/` are clearly live context, while archive and walkthrough material should stay untouched unless they claim current truth.
- The repo already has unrelated local changes in `PROMPT.md` and `.bb-runtime-env`; this lane must avoid disturbing them.
