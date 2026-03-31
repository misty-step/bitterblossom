# Define a Canary triage sprite

Priority: high
Status: blocked
Estimate: L

## Goal
Define a bitterblossom sprite that plugs into Canary, triages errors and incidents as they arise, and creates GitHub issues with LLM-synthesized context — without chasing stale issues, duplicating work, or going down rabbit holes.

## Non-Goals
- Own triage state in Canary — the sprite tracks its own processing state via Canary's annotations API
- Auto-fix issues — triage produces GitHub issues, not PRs (the ramp-pattern item covers closing the loop)
- Replace human judgment for severity — the sprite proposes severity, humans can override
- Handle multi-repo coordination in this item — one repo per sprite instance

## Oracle
- [ ] Given a new incident in Canary, when the sprite's webhook fires, then the sprite polls Canary's timeline from its last cursor, filters for unannotated incidents, and begins triage
- [ ] Given the sprite triages an incident, when triage completes, then it annotates the incident in Canary (`action: "triaged"`, `metadata: {"github_issue": "#N"}`) and the incident no longer appears in unannotated queries
- [ ] Given the sprite crashes mid-triage, when it restarts, then it polls Canary for unannotated incidents and resumes without duplicate GitHub issues (checks for existing issues by group_hash before creating)
- [ ] Given an incident that has already been annotated as triaged, when the sprite encounters it, then it skips it
- [ ] Given the sprite has been running for 1 hour, when its activity is reviewed, then it has not spent more than 5 minutes on any single triage (time-boxed with escalation)
- [ ] Given the sprite definition, when inspected, then it defines: retry budget (max 2 retries per incident), time budget (5min per triage), staleness window (ignore incidents older than 24h), and dedup check (query GitHub issues before creating)

## Notes
Blocked on: Canary annotations API (canary backlog.d/002-annotations-api.md) and timeline enrichment (canary backlog.d/003-timeline-agent-polling.md).

### Responsibility boundary
- **Canary owns:** error ingest, incident correlation, timeline, annotations storage, webhook delivery
- **This sprite owns:** triage judgment, GitHub issue creation, retry/time budgets, staleness filtering, dedup against GitHub, context windowing, LLM synthesis

### Guardrails (prevent rabbit holes)
- Time-box: 5 minutes per incident triage, then escalate or skip
- Retry budget: max 2 attempts per incident, then annotate as `failed` with reason
- Staleness: ignore incidents opened >24h ago with no annotation (they're cold)
- Dedup: before creating a GitHub issue, query GitHub for existing issues matching the group_hash or error fingerprint
- Context window: sprite receives incident summary + recent timeline events, not the full error history

### Integration pattern
1. Webhook from Canary → sprite wakes
2. Poll `GET /api/v1/timeline?after=<cursor>&event_type=incident.opened,error.new`
3. Filter: skip incidents with `triaged` or `failed` annotation
4. For each unannotated incident: synthesize context via LLM, create GitHub issue, annotate in Canary
5. Persist cursor for crash recovery

### Sprite definition location
`sprites/<name>/` in bitterblossom — AGENTS.md, CLAUDE.md, and skill references following existing sprite conventions (weaver, thorn, fern patterns).
