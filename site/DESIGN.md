# Bitterblossom public-site DESIGN.md

Deviation from the site-kit's usual `cp ... DESIGN.md` step, noted here so it
isn't mistaken for an accident: this repo's root `DESIGN.md` already exists
and is a different, established document — the `noir-ledger` UI design
system for `src/operator.html` (colors, typography, layout, components for
the live operator dashboard). It predates this card and governs a different
surface. Overwriting it would have destroyed real, referenced content for no
reason, so the site-kit's public-site brand contract lives here instead,
scoped next to the `site/` directory it actually governs.

This file is the product's **public marketing site** brand contract — not
the operator dashboard's design system. Keep it short and exact: agents and
humans should be able to update `site/` from this file without inventing a
second design system.

## Brand Voice

- Plain-spoken, concrete, and operator-facing.
- Lead with the user outcome, then the proof.
- Avoid marketing fog, mascot language, and decorative claims.
- Bitterblossom is infrastructure, not a magic autonomy pitch: say what runs,
  what it costs, and what stops it — never "autonomous agents that just work."

## Pitch One-Liner

`Bitterblossom runs recurring agent workloads as durable, budgeted jobs — with a real dead-letter queue, a review gate that blocks bad merges, and a fleet heartbeat, instead of a cron job you have to trust blindly.`

## Fleet Marketing Lock

Operator lock-in: 2026-07-07, `misty-step-936`.

- Homepage H1/tagline: `The event plane for agent workflows.`
- Layout: Mural — one viewport, no scroll, lower-left frosted panel.
- Hero image: `site/assets/hero.jpg`, copied from the fleet production image
  `bitterblossom-hero.jpg`; generated with `gpt-image-1` in the Misty Step
  fresco language.
- Image opacity: `0.22`.
- Footer: mode toggle on the left; right side reads
  `a Misty Step project` with `Misty Step` linked to `https://mistystep.io`
  and an inline GitHub glyph linked to
  `https://github.com/misty-step/bitterblossom`. No Weave link, bare URL,
  email, or copyright line.

## Lucide Mark

- Icon: `flower`
- Reason: selected 2026-07-02 through the fleet-wide icon-logo playground
  (`aesthetic/prototypes/icon-logo-playground.html`) and named in
  `backlog.d/111-adopt-lucide-flower-as-the-bitterblossom-wordmark-icon.md`
  (open, not yet adopted elsewhere in the repo) — Sprites infra, an MTG
  enchantment that creates tokens every turn: recurring workloads, spun up
  and run durably.
- Rule: the mark is an inline Lucide SVG inside `.ae-app-mark`. No bespoke
  marks, logo images, emoji marks, or colored wordmarks.

## Palette Hooks

The marketing site keeps the Aesthetic default palette. The operator
dashboard's `noir-ledger` identity (root `DESIGN.md`) is a deliberately
distinct, denser system for a different surface — the marketing site doesn't
borrow it.

```css
:root {
  --ae-accent: #2643d0;
  --ae-accent-dark: #8c9eff;
}
```

## Screenshot Inventory

All three are real captures against a live dev-plane drill
(`BB_DRILL_KEEP_TMP=1 ./scripts/control-loop-drill.sh`, then `bb serve`
against the kept plane) — not mockups:

| File                                                  | Surface                          | State                                                     | Caption                                                                                       |
| ------------------------------------------------------ | ---------------------------------- | ------------------------------------------------------------ | ------------------------------------------------------------------------------------------------- |
| `site/assets/screenshots/01-dashboard-storm-drill.png` | Operator dashboard (`bb serve`)   | Real containment-storm drill: 5 webhook events, 1 success, 4 `blocked_budget` | The event plane refusing to overspend, visibly — not a log line, an operator surface. |
| `site/assets/screenshots/02-status-runs-cli.png`      | `bb status` / `bb runs list` terminal | Same drill state, CLI side                                | The same operator truth from a script or a human's terminal — one ledger, two faces. |
| `site/assets/screenshots/03-loc-tripwire.png`          | `scripts/verify.sh` LOC bloat tripwire | Real count at time of capture: 12,205 of a 12,210-line cap | The spine's own bloat tripwire, at 5 lines of headroom — "the Python conductor died of bloat," and this is the mechanism that keeps the Rust one from repeating it. |

## Footer Links

- Misty Step: `https://mistystep.io`
- GitHub: `https://github.com/misty-step/bitterblossom`

## Release Notes Rule

`site/changelog.html` is user-facing. Write entries as product outcomes, not
commit logs. Each entry needs a date, a version or release label, and one or two
plain-language bullets.

### The v3 rewrite discontinuity (read before editing the changelog)

Bitterblossom has real tagged GitHub releases (`v1.79.0` down to early
`v1.x`), but they belong to the retired Elixir/Python conductor, not the
current Rust `bb` binary. `Cargo.toml` still carries cargo-init's `0.1.0`
placeholder — no version has been cut for the rewrite yet. `docs/release-policy.md`
(`bitterblossom-097`, merged 2026-07-04) defines the semver policy and ships
`scripts/bb-cut-release`, but explicitly leaves the first real v3 version
number as an operator decision, not a default. The changelog says this
plainly instead of pretending `v1.79.0` describes the software that exists
today, and instead of inventing a `v3.0.0` release that hasn't been cut.
