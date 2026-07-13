# LAB-005 usability-first design fence

Artifact: Bitterblossom's authenticated workflow control plane.

This is a clean reroll after the operator rejected LAB-004 in full. Do not repair,
hybridize, or imitate any LAB-004 candidate. Read product truth from `VISION.md`
and `docs/workflow-control-plane.md`; do not read other LAB-005 lane files.

## Binding product hierarchy

1. Configured truth first.
2. A selected workflow's current runtime second.
3. Immutable evidence third.

The landing surface is a quiet configured-workflow roster. Selecting a workflow
opens stable configuration with an optional selected-run overlay. Evidence,
agents, and spend are progressively disclosed. There is no global dashboard and
no wall of summary cards.

## Usability floor

- One primary object and one primary action per view.
- No more than two information planes visible at once.
- At 390px, one task per view; no squeezed desktop composition.
- Native full viewport, not an app inside a decorative page or iframe shell.
- Light and dark are equal. Semantic text meets AA.
- Misty Step Aesthetic is binding: square geometry, hairlines, ink hierarchy,
  status on glyphs, 13px chrome, no gradients, shadows, pills, or card mosaics.
- Use the corrected Aesthetic branch from PR #29 for flow viewport, progress,
  semantic contrast, and mobile rail behavior.
- Controls must work. No meta-copy, fake production claims, or design commentary
  appears inside a candidate.

## Shared truthful corpus

Every candidate renders the same corpus from `../lab-004/data.js`: two configured
workflows, PR Review topology and current execution, distinct execution/domain/
verification/cost truth, agent availability and authority, spend scopes, and the
goal-first creation sequence.

## Lane output

Write exactly one file under `explorations/lab-005/lanes/`. Export `SPECS`, an
array of exactly two declarative holistic-system propositions. Each object has:

- `id`, `label`, `philosophy`, `move`
- `layout`: one of `roster-detail`, `folio`, `command-strip`, `sequence`,
  `split-register`, `focus-stack`
- `roster`: one of `rows`, `index`, `ledger`
- `detail`: one of `topology-first`, `configuration-first`, `run-overlay`
- `navigation`: one of `rail`, `top`, `bottom-contextual`
- `density`: `quiet` or `compact`
- `mobile`: a sentence defining the one-task phone order
- `accent`: `rare` or `active-only`

The two propositions must be structurally distinct, not skins. At least one must
invert a load-bearing layout assumption while preserving the binding hierarchy.
Do not write markup or CSS; presentation belongs to the shared renderer.

## Adjudication

No quota rescue. A candidate is killed if it violates the hierarchy, exceeds two
visible planes, becomes a squeezed desktop at 390px, uses decoration as hierarchy,
or lacks a coherent primary action. Every retained candidate is inspected at
1440x900 and 390x844 in both themes. Weak candidates are refilled before review.
