# Give the plane an agent-facing query API and an operator view

Priority: P2 · Status: pending · Estimate: L

## Goal
Anything an operator or a local agent wants to know about the plane —
what's running, what happened, what it cost, what's parked — is one
HTTP call or one CLI invocation away, plus a minimal human view; and
the two primary workflows have names that stick.

## Oracle
- [ ] `bb serve` exposes a read-only JSON API mirroring the CLI:
      `GET /api/runs?task=&state=`, `GET /api/runs/<id>` (attempts,
      events, artifacts), `GET /api/dlq`, `GET /api/tasks` (parked
      state, budget posture, today's spend) — auth'd with a bearer
      token from plane env, localhost bind by default
- [ ] An agent session (claude/codex on this machine) can answer
      "what did the plane do today and what did it cost?" using only
      that API or `bb --json` — exercised live, transcript captured
- [ ] A single self-contained HTML page served at `/` renders runs,
      states, costs, and parked tasks (no JS framework, no build step —
      the spine's ≤5k LOC budget includes this)
- [ ] The two workflow modes have ratified names used consistently in
      README, docs/spine.md, and CLI help; "Mode A/Mode B" and
      "event-driven vs ad-hoc" prose is swept

## Children
1. Read-only API on the existing tiny_http loop (no new dependency).
2. The HTML view (one templated page, server-rendered).
3. Naming ratification + docs/CLI sweep. Proposal to ratify:
   **reflex** runs (standing, trigger-fired: webhook/cron — the plane
   reacts without judgment) vs **dispatch** runs (deliberate, operator-
   or agent-initiated from a terminal). Alternates considered:
   standing/ad-hoc (accurate, beige), patrol/errand (cute, strained).

## Notes
Operator direction 2026-06-10: primary consumption is agents querying
the plane; a human UI is secondary but wanted. The CLI's `--json` is
already agent-consumable for local sessions; the API matters once the
plane runs detached (deployed near the sprites). Keep it read-only —
mutations stay on the CLI where the ledger's process model is simple.
