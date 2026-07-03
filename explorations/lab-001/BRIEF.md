# LAB-001 — Bitterblossom operator UI redesign

Round 1. Operator verdict pending. This brief is the lane card for every
builder in this lab: read it fully before writing code.

## Intent (one sentence)

The operator opens this UI to answer **"does anything need me right now, and
if so, what exactly do I do?"** — everything else (tasks, runs, money,
health) is drill-in, not front page.

## Operator critique of the shipped dashboard (2026-07-03, verbatim themes)

- Redundancy: budget appears 3× on overview (status line, BUDGET strip, COST
  TODAY card); DLQ count 3×; health/INGRESS duplicates the runs page.
- Empty FLEET table consumes ~half the viewport when there are no leases.
- NEXT ACTION is a wall of raw error dumps; colors unexplained; two cards are
  the same root cause. Unclear what each card *is*.
- Jargon without teaching: DLQ, leases, ingress, "operator work", "blocked
  budget", "38 triggers / 32 manual".
- 32 tasks rendered as a flat alphabetical wall; really ~12 families × model
  variants (`review`, `review-glm`, `review-kimi`, `review-deepseek`).
- Scroll everywhere. Wanted: **full-viewport, no page scroll — pagination /
  view-switching inside fixed panes.**
- "I usually want to do one thing. Make the one thing obvious; make the rest
  subtle punch-outs."
- Fold in the brand mark (the flower). See `icons.html` in this dir.

## Ground truth (real data, use it verbatim — no lorem ipsum, no invented numbers)

Snapshots from the production plane live in `data/*.json` (status, tasks,
runs, dlq, leases, ingress — captured 2026-07-03). Load via `data.js`
(`window.BB_DATA`), built by the scaffold lane. Facts your layouts must
survive:

- 32 tasks ≈ 12 families × model-variant suffixes (`-glm`, `-kimi`,
  `-deepseek`). 1 parked (`review`: 32 runs ≥ 20/day cap).
- Triggers: 32 manual (one per task, auto), 2 cron, 4 webhook. Only the 6
  are real automation — do not present "38 triggers" as signal.
- 149 recent runs: 135 are `review` (webhook). States: 49 failure,
  36 success, 34 retired, 30 blocked_budget.
- 1 open DLQ item (canary-triage: repo sync failed, missing remote ref —
  the same root cause appears twice in shipped NEXT ACTION).
- $0.00 spent today of $25.00 cap. 0 leases. 2 critical freshness contracts.
- `status.json` freshness contracts each carry `safe_next_action` — a real
  `bb` command. The domain already thinks in next-actions; use them.

## Constraint set

FIXED (breaking these = noise, not signal):
- DESIGN.md noir-ledger tokens: ink #151515 / paper #fcfcfc, accent #2643d0,
  ok #15714b / warn #8a5f32 / err #a84138, Geist + Geist Mono, radius 0,
  no shadows, no gradients, no decorative color. Dark mode = same roles.
- Full-viewport: option pages are exactly 100dvh × 100vw, **zero page
  scroll**. Density lives inside fixed panes via pagination, tabs, drill-in,
  or collapse. (Baseline OPT-0 is exempt — it shows the shipped scroll.)
- Evidence-forward: never hide cost, failure, DLQ, staleness. Summarize ≠
  suppress: collapsed detail must be one interaction away.
- Real content from `window.BB_DATA` only.
- Keyboard-reachable interactive controls, visible focus, WCAG AA contrast.
- Copy uses the glossary below — plain words first, term second.

VARYING (diverge hard here):
- Information architecture, navigation model, layout, hierarchy, density,
  what the "one thing" is and how the rest is demoted, how the domain model
  is taught, brand-mark treatment.

## Glossary (rewrite — labels teach, IDs stay for grep-ability)

| Shipped term | Use instead (pattern: plain words lead, term follows) |
|---|---|
| DLQ | "Dead letters — events that failed before a run started" (replay/acknowledge) |
| open DLQ: operator work | the actual verb, e.g. "1 dead event to replay" |
| leases | "Active hosts — sprites currently held by a run" |
| ingress | "Events in" |
| blocked_budget | "Over cap — task hit its daily run limit" |
| retired | "Closed by operator, not replayed" |
| freshness 2 critical | "2 states overdue past their watchdog" |
| triggers 38 / 32 manual | "6 automated triggers (2 cron · 4 webhook)"; manual dispatch is a capability, not a count |
| parked | "Paused by budget — unparks tomorrow or via `bb task unpark`" |

## The seven options (SECTION OVR — overview + IA)

Stable IDs. Structurally distinct — different organizing metaphors, not
palette swaps. Every option must (a) make one primary element unmistakably
dominant, (b) demote everything else to subtle affordances, (c) show the
brand mark once, small, in chrome, (d) handle the empty-fleet case without
wasting space, (e) offer at least one live drill-in interaction.

- **OPT-0 — baseline (shipped).** Faithful reproduction of the current
  overview (rail, proof strip, stat cards, fleet table, next action, recent
  runs). Scrolls, redundancies intact. Round-1 reference, unjudged.
- **OPT-1 — the queue.** Inbox metaphor: one triaged attention item at a
  time, full-width center stage — root-caused, deduped (the two canary-triage
  dead-letter cards become ONE item: "replay after ref fixed"), each with its
  one `bb` command and resolve/skip. j/k or arrows to step, "3 need you"
  counter. All plane state compressed to one thin status bar. Sacrifices:
  ambient overview breadth.
- **OPT-2 — the day ledger.** Accounting-book metaphor: today as a closing
  ledger — one money line (single place budget appears), runs by outcome
  with paired deltas vs yesterday, exceptions listed like reconciliation
  items. Fixed panes, per-pane pagination. Sacrifices: real-time feel.
- **OPT-3 — the roster.** Agent-centric: task FAMILIES as cards in a fixed
  grid (12 families, model-variant chips inside), each with outcome
  sparkline, budget meter, trigger badges; parked/failing families float to
  front with their next action inline. Attention = a badge layer on the
  roster, not a separate panel. Drill-in: click family → its runs, paged, in
  a side pane. Sacrifices: cross-family chronology.
- **OPT-4 — the plane.** Teach-the-domain layout: fixed left→right lifecycle
  columns (EVENTS IN → TRIGGERS → RUNS → EVIDENCE), entities flow, DLQ is a
  visible holding pen hanging off the ingress→run edge, leases a small gauge
  on the RUNS column. Counts + last-3 items per column, paged. The IA *is*
  the glossary. Sacrifices: density of any single entity.
- **OPT-5 — the console.** CLI-first skin over `bb`: persistent command bar
  (real completions from data: `runs show <id>`, `dlq replay 1`,
  `task unpark review`), instrument header strip, and a single output pane
  that renders whichever query ran. Every UI element is a saved query;
  clicking anything writes the equivalent `bb` command into the bar first.
  UI as honest skin over CLI truth. Sacrifices: discoverability for
  non-CLI operators.
- **OPT-6 — the wire (inversion).** Challenges tables-primary: a
  chronological narrated feed, newest first, one plain sentence per event
  ("19:42 review refused — over cap (20/day) · unpark"), consecutive
  same-cause events auto-grouped with counts ("×6 over cap 19:03–19:42"),
  severity as left rules not backgrounds. Fixed viewport, paged by time
  window. Sacrifices: at-a-glance aggregates.

## File contract (collision-free parallel build)

Lab-registry layout, zero-build, self-contained, no external requests:

```
explorations/lab-001/
  index.html   # shell: top bar (viewport presets + custom size), sidebar
               # registry, iframe. Arrow keys switch options. localStorage.
  app.js       # SECTIONS manifest (OVR, opts 0-6, round status, notes)
  styles.css   # shell styles only
  frame.html   # loads tokens.css, data.js, parts.js, then options/opt-*.js,
               # then frame.js; <div id="mount">
  frame.js     # SPECS map assembled from window.BB_OPTS; renders by hash;
               # ignores unknown hashes; demo links href="#0"+preventDefault
  tokens.css   # DESIGN.md tokens as CSS custom props + dark scheme
  data.js      # window.BB_DATA = {status,tasks,runs,dlq,leases,ingress}
  parts.js     # shared builders: caption band, proof strip, stat, table,
               # pager, status bar, mark SVG (flower placeholder)
  options/opt-0.js … opt-6.js   # ONE FILE PER OPTION; registers into
               # window.BB_OPTS['OPT-N'] = {title, notes, build(mount)}
  data/*.json  # raw snapshots (already present)
  icons.html   # mark catalog (separate lane)
  BRIEF.md     # this file
```

Builders: touch ONLY your own `options/opt-N.js` files. Shell/scaffold lane
owns everything else. Version asset URLs (`?v=1`) against stale caching;
bump on each round.

## Quality bar (every option, before you report done)

Zero console errors; renders at 1440×900, 1280×800, and 390×844 (mobile may
stack but still no page scroll); interactions exercised; dark scheme via
`prefers-color-scheme` honored by tokens.css; no external network requests;
`prefers-reduced-motion` respected (motion only on direct interaction
anyway). Report: which option IDs, screenshot paths if captured, and any
brief deviation with a one-line reason.
