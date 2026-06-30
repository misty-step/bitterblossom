# Make Cerberus invoke the Thermo-Nuclear maintainability lens for ship-bound code

Priority: P1 · Status: ready · Estimate: M

## Goal

Ensure every Cerberus/submission-storm review of ship-bound code includes Harness Kit's synced Cursor Thermo-Nuclear Code Quality Review lens, so structural maintainability blockers are found before merge rather than as an optional local ritual.

## Problem / Dogfood Evidence

During the 079 artifact CLI dogfood cycle, the local Cursor Thermo-Nuclear review caught real issues after the builder branch was already green:

- `artifacts list` and `artifacts read` had drift between binary classification rules;
- list/read docs implied broader coverage than the implementation provided;
- CLI JSON error paths for binary/oversized outcomes lacked integration coverage;
- after fixes, re-review caught a blocking read-path IO error swallow that normal tests and the initial fix missed.

The skill exists in Harness Kit at:

```text
/Users/phaedrus/Development/harness-kit/skills/.external/cursor-thermo-nuclear-code-quality-review/SKILL.md
```

But Bitterblossom's Cerberus/review factory does not yet make that lens a required storm member or required reviewer instruction. If the operator forgets to run it manually, BB can ship structurally messy code even when correctness/security/product lanes pass.

## Oracle

- [ ] Cerberus/review/submission-storm task cards or wrapper config explicitly include the Thermo-Nuclear maintainability lens for meaningful implementation diffs.
- [ ] The lens is sourced from Harness Kit's synced skill or a pinned/exported copy with provenance, not retyped drift-prone prose.
- [ ] Reviews distinguish blocking structural regressions from advisory style/nit feedback.
- [ ] The submission gate records whether the Thermo-Nuclear lens ran, passed, blocked, or was intentionally waived with reason.
- [ ] A fixture review diff with a structural maintainability flaw is blocked by the lens.
- [ ] A docs-only or tiny config-only diff can skip the lens only by explicit risk-tier rule.
- [ ] `./scripts/verify.sh` passes.

## Verification System

- Claim: ship-bound implementation diffs cannot clear Cerberus/submission-storm review without either running or explicitly waiving the Thermo-Nuclear maintainability lens.
- Falsifier: a Rust implementation diff reaches `bb gate clear` with no record of the lens; a structural-spaghetti fixture passes; the lens prose drifts from Harness Kit; or reviewers report vague style nits instead of actionable structural findings.
- Driver: local submission-storm fixture with a seeded maintainability flaw, plus a clean artifact CLI-style fixture.
- Grader: flawed fixture blocks with a precise structural finding; clean fixture clears; gate JSON exposes lens receipt.
- Evidence packet: submission id, per-member artifact/REPORT, gate JSON, and the skill provenance path/ref.
- Cadence: every Cerberus/review-task change and before enabling autonomous merge loops.

## Promotion Metrics

This is a review-authority requirement, not an autonomy feature by itself. It gates Level 3 merge autonomy:

- three consecutive implementation branches include a Thermo-Nuclear receipt before merge;
- zero merges require post-hoc local-only Thermo review because Cerberus omitted it;
- structural findings are fixed or explicitly ticketed before gate clear;
- review runtime/cost remains within the storm budget or the lens gets a dedicated budget line.

## Notes

Harness Kit already knows the lens in `skills/code-review/SKILL.md` and `skills/deliver/SKILL.md`; Bitterblossom needs to project that discipline into Mode B review factory execution.
