---
colors:
  surface: "#fcfcfc"
  panel: "#ffffff"
  wash: "#f1f1ef"
  ink: "#151515"
  muted: "#737373"
  faint: "#a3a3a3"
  line: "#d9d9d6"
  frame: "#202020"
  primary: "#2643d0"
  accent: "#2643d0"
  ok: "#15714b"
  warn: "#8a5f32"
  err: "#a84138"
typography:
  fontFamily: "\"Geist\", \"Helvetica Neue\", Helvetica, Arial, sans-serif"
rounded:
  none: "0px"
spacing: ["4px", "8px", "12px", "16px", "24px", "32px"]
---

# Overview

Bitterblossom's operator surfaces use the `noir-ledger` flavor: dense,
read-mostly, square-framed, and evidence-forward. The surface should feel like
an operations ledger, not a marketing dashboard.

The primary maintained surface is `src/operator.html`, served by `bb serve`.
It reads the same JSON truth as the CLI and must never make cost, failure,
readiness, DLQ, lease, ingress, or notification state harder to inspect.

## Colors

The palette is neutral ink and paper with one blue action accent plus semantic
green, amber, and red. Use muted gray for secondary labels. Do not introduce a
purple gradient, glass layer, decorative blob, or one-off status color.

Dark mode mirrors the same roles: near-black surface, dark wash, light ink,
and the same semantic meanings. Do not swap semantic colors between modes.

## Typography

Use the existing sans stack for readable prose and `Geist Mono` or the system
monospace stack for operator chrome, IDs, costs, state, and tabular numbers.
Dashboard headings stay compact. Hero-scale type does not belong inside BB
operator panels.

## Layout

Operator pages are high-density workbench surfaces:

- Left rail plus main desk on desktop; single-column stacked surface on mobile.
- Summary metrics form a ledger grid with 1px dividers.
- Tables remain the primary scan surface for runs, leases, tasks, ingress, and
  dead letters.
- Proof strips expose real plane facts such as ledger schema, budget, notify
  outbox state, and freshness contracts.

## Elevation & Depth

No soft shadows are part of the system. Separation comes from hard borders,
ledger ruling, caption bands, and spacing.

## Shapes

Use hard square panels. The durable radius token is `none: 0px`; any rounded
shape needs a local reason and should not become the dashboard default.

## Components

- Summary metric: label, tabular value, optional meter, muted context line.
- Proof strip: compact bordered row of real state facts, never decorative
  filler.
- Caption band: monospace uppercase heading band at the top of a panel.
- Data table: monospace, tabular, dense rows, visible empty state.
- Action button: square, high-contrast, only for direct commands.
- Quiet button: square outline, secondary operator action.

## Do's and Don'ts

- Do keep costs, tokens, state, stale classifications, DLQs, and notifications
  visible.
- Do use real ledger/API values in proof strips and receipt surfaces.
- Do preserve keyboard-reachable controls and visible focus states.
- Do not hide failure behind illustrations, cards-within-cards, or decorative
  color.
- Do not add ambient animation. Motion is allowed only for direct interaction
  or meter value changes.
- Do not import a JS design package into this Rust static surface unless the
  repo adds a maintained frontend build step.
