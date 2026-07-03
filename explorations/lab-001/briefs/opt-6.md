# Packet: LAB-001 OPT-6 "the wire" — operator UI prototype

One shaped slice: author ONE new file, `explorations/lab-001/options/opt-6.js`,
on your working branch. Nothing else changes.

Read `explorations/lab-001/BRIEF.md` in this checkout first — it is the
binding contract (constraints, glossary, real-data rules, file layout,
quality bar). The scaffold (`frame.html`, `tokens.css`, `data.js`, `parts.js`)
already exists on this branch; your file registers into it:

```js
window.BB_OPTS['OPT-6'] = { title: 'the wire', notes: '…', build(mount) { … } };
```

## The option: OPT-6 — the wire (inversion of tables-primary)

A chronological narrated feed of plane activity, newest first. Structure:

- One plain-language sentence per event built from `window.BB_DATA.runs`,
  `.dlq`, and `.status` freshness contracts: e.g. "19:42 review refused —
  over cap (20 runs/day) · unpark", "05:32 canary-triage event dead-lettered —
  repo ref missing · replay". BRIEF.md glossary voice: plain words lead,
  term follows.
- Consecutive same-cause events auto-group with counts and a time range
  ("×6 review over cap, 19:03–19:42"), expandable in place.
- Severity = 2px left rules in the semantic colors (ok/warn/err), never
  background fills.
- A thin top strip: the one money line, counts, "N need you" — clicking it
  filters the wire to attention items only.
- Items needing the operator carry their one `bb` command inline,
  copy-on-click.
- Exactly 100dvh × 100vw, zero page scroll: the feed pages by time window
  (older/newer + PageUp/PageDown), it does NOT free-scroll.
- noir-ledger tokens from tokens.css only; radius 0; no shadows; monospace
  for times/ids/commands.
- Keyboard: j/k move item focus, e expands a group, PageUp/Down pages.
  Visible focus states. WCAG AA contrast.

Verify by opening `explorations/lab-001/frame.html#OPT-6` (python3
http.server or file://) — zero console errors, renders at 1440×900 and
390×844 without page scroll.

## Output

Commit the one file (`lab-001: opt-6 wire prototype`), push per your
commission. ALSO print the complete final file content to stdout between
`===OPT6-BEGIN===` and `===OPT6-END===` markers, then a 5-line self-review
(works / rough / would-do-next). If anything blocks you, say exactly what on
stdout — never fail silently. Skip `./scripts/verify.sh` — this slice touches
only `explorations/` (no Rust); note the skip in REPORT.json instead.
