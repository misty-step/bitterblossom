# LAB-006 verification

Verified 2026-07-13 in the isolated `design/converged-ui-prototype` worktree.

## Browser exercise

Agent Browser exercised the live local artifact at 1440 x 1000 and 390 x 844.
The pass covered:

- Workflows, both standalone workflow definitions, connected execution topology,
  workflow-scoped runs, global Runs, and standalone run detail.
- The expandable reusable agent roster and its authoring-time declaration picker.
- Spend reporting, workflow attribution, governor editing, and saved draft state.
- All seven authoring steps in light and dark, including optional goal proposal
  adoption, trigger selection, new-agent creation, synthetic test gating,
  operator confirmation, and activation.
- Mobile drawer navigation with no bottom bar, no body-width overflow, and no
  closed-drawer keyboard targets.

Two browser-found defects were corrected during the pass: an undefined upstream
strong-line token made topology connectors transparent, and the mobile drawer
remained open after entering workflow authoring. Fresh criticism then blocked
decorative form state, stale revision proof, partial governor persistence,
untruthful superseded-run evidence, inert controls, and a hard-coded new-agent
escape. Each blocker was corrected and replayed in Agent Browser. The final pass
found no page, console, or asset errors.

Screenshots live under `.evidence/design-lab-006/2026-07-13/screenshots/` and
cover every top-level route plus all seven authoring steps across both themes and
both target widths.

## Repository gate

`./scripts/verify.sh`

Result: all gates green, including Rust format, clippy, the complete test suite,
coverage ratchet, plane validation, local golden path, operations and chaos
drills, and the 15,500-line spine tripwire (`src LOC: 15499`). The optional
repo-local dashboard browser smoke reported Playwright absent and skipped as
documented; the dedicated Agent Browser exercise above covered LAB-006.

## Independent criticism

Result: **PASS** after two correction rounds. The final critic inspected the diff
and rendered evidence against the operator verdict, then confirmed that no
blocking finding remained.
