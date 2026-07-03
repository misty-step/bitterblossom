# Packet: LAB-001 OPT-5 "the console" — operator UI prototype

One shaped slice: author ONE new file, `explorations/lab-001/options/opt-5.js`,
on your working branch. Nothing else changes.

Read `explorations/lab-001/BRIEF.md` in this checkout first — it is the
binding contract (constraints, glossary, real-data rules, file layout,
quality bar). The scaffold (`frame.html`, `tokens.css`, `data.js`, `parts.js`)
already exists on this branch; your file registers into it:

```js
window.BB_OPTS['OPT-5'] = { title: 'the console', notes: '…', build(mount) { … } };
```

## The option: OPT-5 — the console

CLI-first skin over the `bb` CLI. Structure:

- A persistent command bar (top or bottom, your call) with real completions
  drawn from `window.BB_DATA`: `runs show <id>`, `dlq replay 1`,
  `task unpark review`, `runs list --state failure`, `status`.
- A compact instrument header strip: today's spend vs cap (the ONLY place
  money appears), runs-by-state counts, dead letters, active hosts, overdue
  watchdogs — each cell is a saved query.
- One output pane rendering whichever query ran. Clicking an instrument cell
  first writes the equivalent `bb` command into the bar, then renders the
  result from BB_DATA (simulated locally — static prototype, no network).
- Every rendered row is itself actionable: selecting a run writes
  `runs show <id>` into the bar, etc. The UI teaches the CLI by using it.
- Exactly 100dvh × 100vw, zero page scroll; the output pane paginates
  internally (parts.js has a pager).
- noir-ledger tokens from tokens.css only — no new colors, radius 0, no
  shadows, monospace chrome.
- Keyboard: `/` focuses the bar, Enter runs, Tab accepts completion, arrows
  navigate result rows. Visible focus states. WCAG AA contrast.

Verify by opening `explorations/lab-001/frame.html#OPT-5` (python3
http.server or file://) — zero console errors, renders at 1440×900 and
390×844 without page scroll.

## Output

Commit the one file (`lab-001: opt-5 console prototype`), push per your
commission. ALSO print the complete final file content to stdout between
`===OPT5-BEGIN===` and `===OPT5-END===` markers, then a 5-line self-review
(works / rough / would-do-next). If anything blocks you, say exactly what on
stdout — never fail silently. Skip `./scripts/verify.sh` — this slice touches
only `explorations/` (no Rust); note the skip in REPORT.json instead.
