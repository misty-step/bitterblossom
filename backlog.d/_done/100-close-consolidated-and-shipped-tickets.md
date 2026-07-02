# Close the shipped and consolidated tickets cluttering the active queue

Priority: P2 · Status: done · Estimate: S

## Goal
The active backlog reflects reality: tickets shipped or consolidated tonight are archived with pointers, so no future lane re-works closed scope.

## Oracle
- [x] 089 (fails-visibly epic; all boxes checked, PRs #878-#882 merged) archived per the repo's convention.
- [x] 092 (product/instance split; PRs #887-#888, plane excision live) — run its remaining verify.sh checkbox, check it, archive.
- [x] 083 and 087 closed with an explicit pointer to 089's Notes/Children (which consolidated them via #873/#874 + tonight's PRs) — their own unchecked boxes are stale, not open work.
- [x] 091 left ACTIVE (2 real boxes remain: iteration/wall-clock caps + final verify.sh) — do not archive.

## Notes
Mechanical tidy, evidence in the 2026-07-01 runway verification. Also: do NOT pick up 085 (Hermes-based supervisor) — its design premise assumes a Hermes cron runner that was decommissioned fleet-wide 2026-06-30; flagged for an operator rewrite decision in the morning.
**Why:** runway-verification lane, 2026-07-01 — four stale-done tickets inflate the queue and 083/087's unchecked boxes invite duplicate work.

## Delivery Notes

Delivered 2026-07-02.

- Verified PRs #873, #874, #878, #879, #880, #881, #882, #887, and #888 are
  merged through GitHub.
- Archived `083`, `087`, `089`, and `092` with closure notes and pointers.
- Left `091-per-agent-governance.md` active.
- Left `085-hermes-bb-dogfood-supervisor.md` untouched pending operator
  rewrite decision.
- Verification: `./scripts/verify.sh`.
