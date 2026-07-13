# LAB-005 verification

Verified 2026-07-13 in the isolated `design/lab-005-usability` worktree.

## Fence

- Configured workflow truth is the landing surface.
- A selected runtime replaces configuration; immutable evidence replaces runtime.
- The roster is quiet: active and draft are lifecycle states, not execution theater.
- Each view has one primary object and at most one primary action.
- Desktop uses no more than two content planes; phone presents one ordered task.
- Candidate markup and interaction come from one shared renderer. Lanes declare structure only.
- Every candidate uses the Misty Step Aesthetic stylesheet pinned to hotfix commit `2bf1d8a`.
- Light and dark themes are equal requirements.

## Bench and catalog

Six blind design philosophies produced two declarations each: Precision Instrument, Editorial Calm, Mobile Task Flow, Spatial Sequence, Operations Ledger, and Radical Restraint. The resulting twelve structural tuples are unique:

1. `precision-register`
2. `precision-focus`
3. `editorial-register`
4. `editorial-focus`
5. `mobile-focus-stack`
6. `mobile-definition-sequence`
7. `spatial-atlas`
8. `spatial-run-path`
9. `operations-command-ledger`
10. `operations-topology-register`
11. `restraint-workflow-folio`
12. `restraint-context-command`

The first independent verdict blocked state truth, duplicate structures, mobile collapse, and declaration/behavior mismatches. The catalog was refilled and the shared renderer was corrected before this final verification.

## Rendered exercise

Playwright exercised 48 cases: twelve candidates, light and dark themes, 1440 x 900 desktop and 390 x 844 phone. Every case traversed the workflow landing, configured truth, selected current run, workflow-scoped evidence, global Runs, Agents, Spend, and Create. The exercise also checked failed assets, page errors, body overflow, and horizontally clipped controls.

Result: `PASS 48/48 rendered interaction cases`.

A separate 390 x 844 exercise selected the inactive Canary Resolution workflow. Its runtime title read `Canary Resolution · no active run`, and its evidence ledger contained Canary Resolution only, excluding PR Review.

Result: `PASS inactive workflow title and selected-only evidence`.

Twenty-seven screenshots were regenerated after the final corrections with reduced motion, `networkidle`, `document.fonts.ready`, and a one-second settle: light desktop and dark phone for every candidate, plus configuration/run/evidence state captures.

The published catalog at `https://sanctum.tail5f5eb4.ts.net/artifacts/a/bb-lab-005/` returned HTTP 200. A fresh 390 x 844 browser session loaded the published modules and Aesthetic asset, selected PR Review, opened its current run, and found no page errors, failed responses, console defects, or body overflow.

## Repository gate

`./scripts/verify.sh`

Result: all gates green, including Rust format, clippy, tests, coverage ratchet, plane validation, local golden path, operations and chaos drills, and the 15,500-line spine tripwire (`src LOC: 15499`). The optional local dashboard browser smoke reported Playwright absent from the repo-local install and was skipped as documented; the dedicated LAB-005 Playwright exercises above ran against the installed browser toolchain.

## Upstream Aesthetic dependency

LAB-005 consumes `https://cdn.jsdelivr.net/gh/misty-step/aesthetic@2bf1d8a/aesthetic.css`. That upstream hotfix corrects semantic contrast, mobile rail disclosure, neutral progress treatment, and gallery contrast coverage. Its own `npm run ci` passed 26 unit and 95 Playwright tests; focused independent criticism passed 20/20.

## Verdict

Fresh-context critic: **PASS**. The critic confirmed truthful state separation, selected-workflow evidence scope, declaration/behavior alignment, twelve distinct structures across six layout families, correct individual dark-phone captures, light/dark parity, strict Misty Step usage, and a reviewable published artifact. No remaining defect warrants killing or refilling a candidate before operator review.

## Catalog discoverability correction

Operator review found that the twelve designs existed but the catalog exposed only one discoverably: candidate navigation was hidden behind an unexplained `LAB` button. The correction replaces it with a visible `Design N / 12` switcher, immediate previous/next controls, and a named twelve-option selector. It also restores Bitterblossom's canonical Lucide `flower` inside Aesthetic's `.ae-logo` / `.ae-app-mark` contract; phone chrome keeps the flower and elides only the wordmark text.

The full rendered journey was repeated after this correction.

Result: `PASS 48/48 rendered interaction cases with discoverable catalog and flower mark`.

The twenty-four per-candidate light-desktop/dark-phone captures were refreshed, and dedicated desktop/phone selector captures were added. Targeted assertions prove the flower mark, visible count, previous/next URL transition, twelve named selector options, candidate selection, asset health, and zero visible-control or body overflow at 1440 x 900 and 390 x 844.

Fresh criticism initially blocked the open phone selector because its short drawer exposed underlying workflow fragments. The phone selector now owns the complete workspace between the 52px product chrome and 50px bottom navigation, scrolls independently, and exposes no candidate content until closed. Geometry, hit-testing, and clipping assertions pass; the critic's final verdict is **PASS**.
