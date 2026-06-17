# Make operator read APIs easier to summarize correctly

Priority: P2 | Status: ready | Estimate: S

## Goal

Reduce agent/operator mistakes when comparing `bb gate --json`,
`bb runs list --json`, and `/api/submissions` evidence during review-storm
dogfood.

## Oracle

- [ ] `/api/submissions` exposes the submission's key identity fields
      (`id`, `change_key`, `rev`, `state`, `round`) in a documented,
      low-friction place while preserving existing detailed verdict rows.
- [ ] `docs/spine.md` names the exact shape or gives a one-line `jq` recipe for
      summarizing submission rev/state.
- [ ] A read-API contract test protects the shape used by agents.
- [ ] `./scripts/verify.sh` passes.

## Notes

Dogfood source: during the 2026-06-17 PR-reflex-storm local `bb serve` drill,
the first evidence summarizer failed because `bb gate --json` exposes top-level
`rev`, while `/api/submissions` returns a nested `submission.rev`. The data was
correct, but the shape made it too easy for an agent to write the wrong
summarizer on the first pass.

Do not add a dashboard here. This is a read-contract cleanup: either flatten a
small summary layer or document the nested shape well enough that evidence
scripts do not guess.
