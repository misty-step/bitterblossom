# Escalation & Composition (the agent at the core)

Operator-ratified 2026-07-06.

**Composition:** the agent is the core; every tool reaches it as a face — MCP,
or a skill coupled with a CLI/SDK/API. No middleware between agents and their
tools: when your question gets answered, YOU write the ruling back to the card
trail with your own powder MCP. Applications stand alone; weave composes them.

**Roster is composition-free** (operator ruling 2026-07-07): roster declares
who CAN exist — a flat catalog of identities other applications compose. WHO
REPORTS TO WHOM is declared per-workflow in bitterblossom's workflow config
(trigger + agent + supervisor chain). Do not encode org structure in roster.

**Escalation hierarchy:** never skip a tier.
1. Session subagents escalate to their orchestrator — never to the operator.
2. The orchestrator answers everything it can; it loops the operator in only
   when the decision is genuinely the operator's (taste, money, irreversible
   scope, external identity).
3. Unsupervised workflow agents (PR reviewers, incident triage, scheduled
   sweeps) escalate to the standing boss agent — frontier intelligence that
   reads full context, rules when evidence supports it, and escalates to the
   operator only with good reason, carrying a tight zero-context packet.

Every ask, ruling, and escalation is logged in full — decision points are
EvidencePack sources and appear in the operator's window reports. An
unlogged decision didn't happen.
