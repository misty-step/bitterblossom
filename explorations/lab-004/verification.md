# LAB-004 verification receipt

Observed 2026-07-13 14:08 CDT / 2026-07-13 19:08 UTC.

- Registry: 12 retained candidates plus the shipped baseline. Source:
  `app.js`; duplicate and corpus-break exclusions are recorded in `brief.md`.
- Browser matrix: 26 candidate/viewport cases exercised at 1440×900 and
  390×844. Every case retained the required corpus truths, a semantic heading,
  a working light/dark control, and no document-level horizontal overflow.
  Source: live `agent-browser` DOM readback against the local HTTP surface.
- Visual evidence: four representative desktop/mobile and light/dark captures
  live under `.evidence/design-lab-004/2026-07-13/`.
- Fresh critique: Anthropic split-desk collision and the fixture badge covering
  mobile navigation were blocking; both were corrected and the full matrix was
  rerun. The critic ranked `ANTH-1`, `MIN-2`, and `APPLE-2` strongest among the
  four representative renders it inspected.
- Aesthetic source scan: no raw consumer colors, decorative gradients,
  shadows, thick accent side-tabs, or looping animation remain. `impeccable`
  reports one false-positive black default on the shell title because its
  isolated detector does not resolve the external pinned Aesthetic stylesheet;
  browser readback resolves the title to `--ae-ink` (`rgb(21, 21, 21)`).
- Kit feedback: `aesthetic-running-status` records the independently observed
  need for a neutral executing/running glyph distinct from warning, success,
  and error. No other kit gap was promoted from consumer composition into a
  Powder card.
- Repository gate: `./scripts/verify.sh` passed, including tests, the coverage
  ratchet, both plane checks, smoke drills, and the 15,499-line spine tripwire.
- Published readback:
  `https://sanctum.tail5f5eb4.ts.net/artifacts/a/bb-lab-004/` returned HTTP 200;
  the live browser exposed all 13 registry options and the baseline iframe.
