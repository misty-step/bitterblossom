# Reviewer Evidence

## Claim

This branch turns a successful conductor builder pass into one explicit operation. The governor no longer has to remember, in six separate branches, to both restore `phase=awaiting_governance` and record the matching builder event.

## Before

- `scripts/conductor.py` duplicated the same `run_builder(...) -> update_run(...) -> record_event(...)` sequence in the initial builder path, review revisions, CI revisions, PR-thread revisions, external-review revisions, and final polish.
- Any future change to builder handoff semantics needed to touch every copy in the governor loop.
- The invariant "a successful builder turn returns control to governance with fresh PR metadata and an event log entry" lived in call-site convention instead of one module boundary.

## After

- `run_builder_turn(...)` now owns the successful handoff contract for governance-managed builder work.
- `run_once(...)` and `govern_pr_flow(...)` call that boundary instead of re-spelling the postconditions.
- `scripts/test_conductor.py` now includes a focused regression test proving that a builder turn updates run state and emits the expected event.

## Why This Matters

The conductor is the judgment-heavy part of Bitterblossom, so its hot path should hide repeated sequencing details, not leak them into every branch. This refactor removes change amplification from the most central loop without changing behavior or broadening the transport surface.

## Artifact

- Walkthrough notes: `docs/walkthroughs/builder-turn-handoff.md`
- Renderer: diagram + terminal evidence

## Evidence Bundle

- `scripts/conductor.py`
- `scripts/test_conductor.py`
- `docs/walkthroughs/builder-turn-handoff.md`

## Protecting Checks

- `pytest -q scripts/test_conductor.py`
- Targeted governance slice:
  `pytest -q scripts/test_conductor.py -k 'run_once or govern_pr'`

## Residual Gap

`scripts/conductor.py` is still a large single module. This branch removes one high-churn seam inside it; it does not attempt the riskier multi-file conductor split.
