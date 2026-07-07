---
name: design
description: |
  Artifact-backed interface design: critique, polish, redesign, generate, and a
  repo-owned design contract. One front door over a bench of design specialists —
  routes to exactly one primary per role; you pick the aesthetic.
  Requires screenshot, URL, rendered artifact, or explicit file plus intent.
  Use when: "make this look better", "improve the design", "polish the UI",
  "critique this screen", "design pass", "art direction", "make it premium",
  "make it brutalist/minimalist", "deslop this", "scaffold design", "DESIGN.md",
  "design system", "prototype this", "show me a few options", "mock up
  variations", "is this accessible", docs layout, report polish, generated
  diagrams/images, dashboards, charts, or any product-facing visual artifact.
  Trigger: /design, /prototype.
argument-hint: "[audit|polish|redesign|scaffold|prototype|<preset>|<verb>] <artifact-or-surface>"
---

# /design

Critique and improve a rendered artifact against its intent — and route the
specifics to the right specialist. `/design` is one front door over a bench of
design skills: it owns the contract and the taste call and dispatches each job
to exactly one primary per role. It is a **menu, not a pipeline** — every route
below stands alone; none waits on another's output.

## The core contract (every route)

These six are the method *within* a single design pass — how one route executes,
not a sequence across routes. Pick the route that fits the job and run this once.

1. Name the artifact: screenshot, URL, rendered file, route, or the source file
   that produces the surface.
2. Name the intent in one sentence: audience, job, desired feel.
3. Inspect the rendered result when possible.
4. State the design read: surface kind, audience, desired feel, constraints.
5. Set the dials (below) for the surface.
6. Return ranked, specific moves — or implement a bounded change — then verify
   the render.

Refuse to make a final design judgment from code alone when a rendered surface
can be inspected. If rendering is impossible, mark the design unverified.

## Set two dials first (choices, not steps)

**Aesthetic preset** — pick the vibe. Each is a distinct, vendored direction,
not a restyle of one template:

| Preset | Feel | Primary |
|---|---|---|
| default | neutral premium — Zinc/Slate, restrained | `leon-taste-skill` |
| soft | glossy, luxe, agency-tier | `leon-soft-skill` |
| minimalist | flat editorial (Notion/Linear), no gradient/shadow | `leon-minimalist-skill` |
| brutalist | raw industrial-terminal, `border-radius:0`, hazard accent | `leon-brutalist-skill` |

For greenfield generation, `nutlope-hallmark`'s genres (editorial / modern-
minimal / atmospheric / playful) are a parallel preset axis — pick one there.

**Variance / Motion / Density** — set per `references/anti-slop.md` before
building. A dashboard and a landing page sit at opposite ends; state the target.

## Route to one primary per role

Each row is an independent entry point. Invoke the one that matches the job; do
not chain them into a sequence.

| Need | `/design` verb | Primary |
|---|---|---|
| Critique / audit a rendered surface | `audit` | inspect + `impeccable audit` (shape: `references/critique-shape.md`) |
| Final meticulous polish pass | `polish` | `jakub-make-interfaces-feel-better` (exact values) + `impeccable polish` |
| Redesign an existing site, keep the stack | `redesign` | `impeccable audit`+`critique`; fold `leon-redesign-skill`'s a11y/SEO omissions checklist; direction menu in `references/taste-layer.md` |
| Generate a page / identity from scratch | — | `nutlope-hallmark` (+ `leon-gpt-tasteskill` for GSAP/AIDA generation) |
| Build UI to match a design image | — | `leon-images-taste-skill` (image-gen runtime is live — `primitives/shared/references/image-generation.md`) |
| Generate an image / diagram / contact sheet of options | — | image-gen direct — NB2 Lite ~$0.03/img, legible in-image text; `primitives/shared/references/image-generation.md` |
| Motion — author it | `animate` | `emil-emil-design-eng` |
| Motion — review it | — | `emil-review-animations` |
| Accessibility / web-interface guidelines | — | `vercel-web-design-guidelines`; pass checklist in `references/interface-polish.md` |
| Page / route transitions (React) | — | `vercel-react-view-transitions` |
| Component architecture / prop API | — | `vercel-composition-patterns` |
| Repo design contract + tokens | `scaffold` / `document` | `@google/design.md` (`lint` / `export dtcg`) via `references/scaffold.md`; `leon-stitch-skill` for Stitch format |
| Typography / color / layout / distill / harden / adapt / clarify / onboard / optimize / delight / overdrive / live | `typeset` … | the matching `impeccable <verb>` |
| Slop detection before shipping | — | `references/anti-slop.md`; superset gate `nutlope` slop-test; deterministic `npx impeccable detect src/` (exit 2) |

The dial and role axes are orthogonal, so a request never matches two rows
ambiguously: an **aesthetic preset** wins when the request names a *vibe*
(brutalist, luxe, minimalist); a **role** wins when it names a *surface to
improve or a job to do* (audit this, polish these, redesign that).

Use `/qa` for behavior verification and evidence capture, and `/shape` when the
product direction itself is unsettled.

## The Design Labs Law (hard gate, no exceptions)

Every route that **creates or redesigns** a surface — generate, redesign,
prototype, scaffold a system, shape a flow — must produce **at least 6
structurally distinct options (6-20, scale to render cost)** in the paged
`references/lab-registry.md` viewer, and keep looping rounds until the
operator locks one in. Never one-shot a single answer. Never auto-pick the
aesthetic — the pick is the operator's, the spread is yours. A "variant" that
is a palette or font swap of one layout does not count toward the 6.

Constraint-naming, the axes that vary by design mode (system / component /
page / UX flow / motion / copy), fan-width calibration within 6-20, and the
converge handoff once the operator picks: `references/divergence.md`. That
reference is method; this gate is the floor it operates under.

## Review gate — earned approval, default-deny

Substantive or external-facing visual changes pass a gate before "done", not a
rubber stamp. Run two fresh-context lanes — a heuristic design-director read and
a deterministic scan (`npx impeccable detect` + the anti-slop quick gate) — and
**synthesize** them; do not just concatenate. Approval is earned. These are
presumptive blockers the author must justify, not nits:

- An off-system value (color / spacing / radius / type) not in the repo's tokens.
- A surviving slop tell from `references/anti-slop.md`.
- Contrast below WCAG AA, or an interactive control unreachable by keyboard.
- No distinctive decision — the surface is template-clean but anonymous.
- Meta-copy: UI that explains the agent's process instead of naming the thing.
- A generative/redesign request answered with fewer than 6 lab-registry
  options, or "variants" that are palette/font swaps of one layout rather than
  structurally distinct directions within the named constraints.

## Delegation

Delegate per the shared Roster contract (shared AGENTS.md: Roster). One lane
owns the proposed direction/implementation; a separate fresh-context lane does
cold review of substantive redesigns, external-facing polish, or final
critique — critics get the artifact and the oracle only, never the author's
reasoning trail (shared AGENTS.md: Fresh context beats self-review). Add
cross-model critics or `/sprites` only when they answer a distinct question.

## Implementation Guardrails

- Change the fewest surfaces that make a coherent improvement; prefer hierarchy
  and content structure over decoration.
- No framework, animation system, or token layer for a one-off surface. Scaffold
  a project-local contract before enforcing tokens across recurring UI.
- Preserve domain truth; polish must not launder weak claims. Keep process out
  of the UI — copy reads as the finished surface, not internal notes.
- After visible changes, verify desktop and mobile render and report evidence.
- If a repo has `DESIGN.md`, read it before any visual change and update it when
  durable facts change; else scaffold it (`references/scaffold.md`) or waive as
  one-off in the completion gate.

## References

- `references/scaffold.md` — repo-owned `DESIGN.md` in the `@google/design.md`
  format (`lint` + DTCG export) and `design-contract.md` provenance.
- `references/design-system.md` — token and component-system judgment.
- `references/taste-layer.md` — aesthetic direction, anti-generic critique,
  and the redesign direction menu (operational, editorial, brand-forward,
  reference/catalog, minimal polish, inversion).
- `references/aesthetic-library.md` — six operator-endorsed aesthetic
  directions (neo-brutalist, terminal-TUI, soft-luxe, memphis, web-1.0,
  instrument-panel), each with DNA + a runnable HTML example to pull from.
- `references/divergence.md` — the divergence-first method behind the Design
  Labs Law: per-mode variation axes, constraint-naming, fan-width calibration,
  and the converge handoff.
- `references/anti-slop.md` — the single ban-core: slop tells, dials, quick gate.
- `references/interface-polish.md` — micro-polish and accessibility checks.
- `references/critique-shape.md` — the critique output shape.
- `references/external-design-references.md` — license-safe use of the vendored
  design skills and inspiration libraries.
- `references/ui-surface-routing.md` — composing `/design` + `/qa` + `/code-review`.
- `references/lab-registry.md` — paged adjustable-viewport prototyping viewer.
- `evals/routing-eval.md` — the routing oracle (this skill's verification system).

## Completion Gate

See `primitives/shared/AGENTS.md` (Completion Evidence) for the shared core; this
phase keeps design-specific fields.

```markdown
**Completion Gate**
- Direction chosen: critique, polish, redesign, generate, or scaffold applied.
- Route taken: which preset and which role-primary were invoked.
- Design read: surface kind, audience, desired feel, constraints.
- Dials: VARIANCE / MOTION / DENSITY values chosen for this surface.
- Evidence that proves it: screenshot, render, artifact, or visual diff inspected.
- Exact command/path/route exercised: URL, screenshot path, render command, or artifact path.
- DESIGN.md status: read, created, updated, not present with waiver, or n/a with reason.
- Hierarchy/content + type/layout changes: the specific issues changed or recommended.
- Copy provenance: visible copy inspected for product truth vs. agent-process leakage.
- Distinctive decision: the intentional choice that prevents template sameness.
- Slop detector: `npx impeccable detect` result on changed files (clean, listed, or n/a).
- Design Labs Law: option count and lab-registry round for any create/redesign route, or n/a.
- Residual risk: remaining design, a11y, or QA risk after inspection.
```

## Gotchas

- Design critique without an inspected artifact is speculation.
- The routes are a flat menu: never sequence them into a phase pipeline. No
  route depends on another route's output — that drift is the workflow engine
  VISION.md forbids.
- Aesthetic preference is not blocking unless it hurts comprehension, trust,
  conversion, accessibility, or domain fit.
- Generic "modernize" moves are slop when they ignore audience, density, or the
  existing system.
- Never hide UI defects behind feature explanations. Point to the visible
  artifact and the concrete change.
- Meta-copy in UI is a design defect: real product policy, privacy, draft-state,
  or compliance copy is fine when the user is meant to see it; leaking the
  agent's caution is not.
