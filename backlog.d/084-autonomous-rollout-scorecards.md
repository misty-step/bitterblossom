# Codify autonomous rollout scorecards and promotion gates

Priority: P1 · Status: ready · Estimate: M

## Goal

Make every lesser-authority shipment — read-only, report-only, dry-run, or PR-only — carry explicit metrics and trigger conditions for deciding whether to promote, hold, or roll back autonomy.

## Oracle

- [x] A reusable rollout scorecard template exists for BB task families: current authority, allowed actions, forbidden actions, evidence metrics, promotion trigger, rollback trigger, budget/cost cap, duplicate-suppression key, and required artifact handles. → `docs/rollout-scorecards.md`.
- [ ] `bb status --json` or a documented artifact/report surface can show the active authority level and scorecard link for autonomous task families. → deferred: the documented surface (card + `docs/rollout-scorecards.md` + report `authority`/`constraints` fields) exists; a structured `bb status --json` authority field needs a spec decision on whether authority is a first-class `task.toml` governance field. Tracked below.
- [x] Backlog 078, 079, 080, 081, 082, and 083 reference concrete promotion metrics rather than vague “later add autonomy” language. → 080/081/082 already carry metrics and now point at the canonical doc; 078/079/083 are done.
- [x] A promotion issue cannot be marked ready unless it cites evidence packets from the lower-authority mode. → codified as doctrine in `docs/rollout-scorecards.md` and the skill.
- [x] The Bitterblossom skill/operator recipe tells agents to refuse autonomy expansion without a green scorecard and explicit operator approval. → `skills/bitterblossom/SKILL.md` "Rollout Scorecards" + `references/operator-recipes.md` "Autonomy Promotion".
- [x] `./scripts/verify.sh` passes.

## Verification System

- Claim: BB can ship low-authority workflows without losing the path to higher autonomy or accidentally promoting by inertia.
- Falsifier: a report-only/read-only ticket lands without metrics; a promotion ticket lacks evidence from prior mode; status/artifacts do not expose authority level; or an agent recommends write authority from vibes instead of scorecard evidence.
- Driver: apply the scorecard to Canary triage, backlog-chewer, artifact/MCP read surfaces, and one fixture autonomous task.
- Grader: each lower-authority issue names measurable promotion and rollback triggers; promotion issue cites run ids/artifacts/gate receipts; operator can see the current authority level from BB surfaces.
- Evidence packet: scorecard template, updated backlog tickets, one filled example for Canary report-only, and command transcript showing the scorecard link in status/artifact output or documented fallback.
- Cadence: before any workflow moves from observe/report/read-only into branch/PR/merge/rollback authority.

## Rollout Scorecard Template

Each autonomous task family gets a compact scorecard:

```text
Task family:
Current authority: read-only | report-only | dry-run | PR-only | guarded-land | rollback-own-change
Allowed actions:
Forbidden actions:
Promotion metrics:
Promotion trigger:
Rollback / hold trigger:
Budget and duplicate caps:
Required artifacts:
Human/operator approval needed for next level: yes/no
```

## Notes

Dogfood source: 077+narrow053 and 079 showed the pattern repeatedly. Read/report-only slices are the right way to start, but only if they are shipped with the telemetry and promotion criteria that tell us when more function or autonomy is warranted.

## Delivery Notes

### 2026-07-02 scorecard template + doctrine slice

- Added `docs/rollout-scorecards.md`: the canonical authority ladder, the
  reusable scorecard template, the promotion/hold/rollback doctrine, and filled
  scorecards for the shipped low-authority task families (canary-triage,
  backlog-chewer-dry-run, fix-prompt-generator, artifact/MCP read surfaces).
- Wired the refusal doctrine into the exportable skill:
  `skills/bitterblossom/SKILL.md` gained a "Rollout Scorecards" section and
  `references/operator-recipes.md` gained an "Autonomy Promotion" recipe. Both
  tell agents to refuse autonomy expansion without a green scorecard plus
  explicit operator approval.
- Pointed backlog 080/081/082 at the canonical doc so their per-ticket
  scorecards stop drifting in shape.
- Proof: `./scripts/verify.sh` (fmt, clippy, tests, `bb check` on all planes,
  LOC budget) green.

Deferred (oracle item 2): a structured authority field in `bb status --json`.
The documented surface exists today (task card + this doc + the report
`authority`/`constraints` fields), but making authority a first-class,
status-visible field is a spine change that needs an operator/spec decision on
whether authority belongs in `task.toml` alongside budget. Left as the remaining
open item on this ticket rather than guessed at overnight.
