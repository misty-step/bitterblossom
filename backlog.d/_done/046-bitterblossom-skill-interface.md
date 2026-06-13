# Add a portable Bitterblossom skill interface

Priority: P1
Status: done
Estimate: S

## Goal

Consuming agents can load a first-class Bitterblossom skill instead of
rediscovering the `bb` event-plane CLI, ledger, dispatch, DLQ, submission, and
parked-task contracts from long project docs every time.

## Oracle

- [x] `skills/bitterblossom/SKILL.md` has trigger metadata for `bb`,
      Bitterblossom, event-plane, run, DLQ, task inventory, submissions, and
      review-factory workflows.
- [x] The skill routes agents to stable `--json` CLI surfaces before dispatch.
- [x] The skill is portable as a whole folder, with `agents/openai.yaml` and
      concrete operator recipes under `references/`.
- [x] A repo test guards the skill artifact against losing core commands,
      install guidance, and the verdict-task payload gotcha.
- [x] `./scripts/verify.sh` passes.

## Closure 2026-06-13

Closed by adding the portable `skills/bitterblossom/` folder and
`tests/skill_artifacts.rs`.

Harness Kit extraction is intentionally not committed as a local symlink in
this slice; the shape packet records the safer next step: add a Harness Kit
source/projection mechanism for product-owned local skills, then point it at
this folder.
