# Make operator read APIs easier to summarize correctly

Priority: P2 | Status: done | Estimate: S

## Goal

Reduce agent/operator mistakes when comparing `bb gate --json`,
`bb runs list --json`, and `/api/submissions` evidence during review-storm
dogfood.

## Oracle

- [x] `/api/submissions` exposes the submission's key identity fields
      (`id`, `change_key`, `rev`, `state`, `round`) in a documented,
      low-friction place while preserving existing detailed verdict rows.
- [x] `docs/spine.md` names the exact shape or gives a one-line `jq` recipe for
      summarizing submission rev/state.
- [x] A read-API contract test protects the shape used by agents.
- [x] `./scripts/verify.sh` passes.

## Notes

Dogfood source: during the 2026-06-17 PR-reflex-storm local `bb serve` drill,
the first evidence summarizer failed because `bb gate --json` exposes top-level
`rev`, while `/api/submissions` returns a nested `submission.rev`. The data was
correct, but the shape made it too easy for an agent to write the wrong
summarizer on the first pass.

Do not add a dashboard here. This is a read-contract cleanup: either flatten a
small summary layer or document the nested shape well enough that evidence
scripts do not guess.

## Delivery Notes

### 2026-07-02

- Added additive top-level `id`, `change_key`, `rev`, `round`, and `state`
  fields to submission bundles while preserving nested `submission`, `verdicts`,
  and `rejections`.
- Extended the versioned agent read-surface contract fixture for
  `submit_list` and `api_submissions`.
- Documented the exact summary shape and `jq` recipe in `docs/spine.md`.
- Focused verification:
  `cargo test --test submission list_submissions_includes_verdict_rows_for_gardener_api`;
  `cargo test --test serve submissions_read_api_exposes_top_level_identity_summary`;
  `cargo test --test agent_contract_fixtures versioned_agent_read_surface_contract_fixture_validates_cli_and_api`.
