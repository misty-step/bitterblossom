# LAB-003 fence and lane contract

Artifact: Bitterblossom's authenticated operator application.

Intent: make the unattended agent fleet understandable as configured workflows,
their triggers and Roster agents, their live and historical instances, their
authority and spend, and their evidence. The create flow is workflow-first and
goal-first.

## Fixed

- Product truth and nouns in `VISION.md` and `docs/workflow-control-plane.md`.
- Primary navigation: Workflows, Agents, Runs, Spend, plus Create workflow.
- Landing is the configured workflow roster, not KPIs or an activity feed.
- Workflow detail uses one stable configured graph and selectable run overlays.
- Workflow state, trigger health, run lifecycle, domain result, and verification
  are distinct.
- Light and dark themes, defaulting to system; WCAG AA; keyboard reach;
  reduced-motion support.
- Zero-build static HTML/CSS/JS. No libraries, remote assets, fake metrics,
  decorative gradients, glass cards, or marketing copy.
- The same `CORPUS` from `data.js` appears in every candidate. It is explicitly
  a design fixture built from ratified product configuration and live Roster
  definitions, not claimed production state.

## Varying

- System proposition: typography, color architecture, density system, shape,
  borders/elevation, graph grammar, hierarchy, navigation treatment, and the
  relationship between roster scan and selected workflow detail.
- Structural layout, not merely palette or font.

## Dials

- VARIANCE: 7/10. Distinctive and structural, never obscure.
- MOTION: 2/10. Only direct interaction and state transitions.
- DENSITY: 8/10. Operationally rich, with progressive disclosure.

## Required gallery inside every option

Every candidate is one full-viewport product proposition and must expose all of
these through visible sections or working in-option navigation:

1. configured workflow roster;
2. PR Review workflow topology and selected live-run overlay;
3. agent roster with In use and Available distinction;
4. live/history run evidence;
5. spend controls and reported/estimated/unavailable truth;
6. goal-first create flow with enhanced goal review and activation test.

Required states: active, draft, trigger listening, executing, succeeded with a
blocked domain result, superseded, verification achieved, unknown cost, and no
active run. Do not collapse them into one traffic-light status.

## Lane output

Write exactly one file: `lanes/<alias>.js`. Export `SPECS`, an object with three
namespaced IDs. Each value has `label`, `move`, `philosophy`, and
`render(corpus)`, returning one complete HTML string including option-scoped
CSS. IDs are stable and never reused.

Each option must include one visible `[data-theme-toggle]` control. The shared
frame handles the click and stamps `data-theme` on `<html>`.

Return three structurally distinct propositions, not three skins. At least one
must invert a load-bearing assumption. End your final message with one line per
option: `ID — structural move`.

## Registry adjudication

The six blind lanes produced 18 raw candidates. The catalog retains 15 plus the
current baseline. Three were removed as structural duplicates before operator
review:

- `ANTH-1` duplicated the persistent roster + graph + evidence-inspector move
  expressed more distinctly by `IMPEC-1` and `BRUT-1`.
- `ANTH-3` duplicated the expandable configuration-ledger move expressed more
  distinctly by `HALL-2` and `BRUT-2`.
- `HALL-1` duplicated the same three-part roster + graph + evidence rail.

No candidate was removed for palette, typography, or subjective taste. Every
blind philosophy remains represented in the 15-option registry.
