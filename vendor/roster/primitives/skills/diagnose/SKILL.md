---
name: diagnose
description: |
  Investigate, audit, triage, and fix. Systematic debugging, incident lifecycle,
  domain auditing, and issue logging. Feedback-loop-first protocol:
  reproduce or replay before root cause, pattern analysis, hypothesis test,
  and fix.
  Use for: any bug, test failure, production incident, error spikes, audit,
  triage, postmortem, "diagnose", "why is this broken", "debug this",
  "production down", "is production ok", "audit stripe", "log issues".
  Trigger: /diagnose.
argument-hint: <symptoms or domain> e.g. "error in auth" or "audit stripe"
---

# /diagnose

Find root cause. Fix it. Prove it works.

## Execution Stance

You are the executive orchestrator: keep hypothesis ranking, root-cause
proof, and fix selection on the lead model; delegate bounded evidence
gathering and implementation to focused subagents. For design-quality root
causes, dispatch an ad-hoc **lens** critic
(`primitives/shared/references/lenses.md`) returning `finding · evidence
file:line · impact` — name the lens, no static agent file.

## Delegation

Delegate per the shared Roster contract (shared AGENTS.md: Roster).

- **Default:** spawn at least two roster-backed Explore lanes to gather
  evidence — reproduce, trace data flow, report root cause + evidence +
  proposed fix. They never implement the fix. You rank the findings, confirm
  root cause is proven, then dispatch a builder.
- **Multi-Hypothesis Mode:** when more than two plausible causes would make a
  single investigation anchor on one, spawn one Explore lane per hypothesis to
  prove or disprove it, then synthesize a consensus root cause before
  dispatching the fix. Use for ambiguous stack traces or multiple services;
  skip for an obvious single cause. `/research delegate` adds multi-model
  hypothesis validation when useful.

## Routing

| Intent | Sub-capability |
|--------|---------------|
| Debug a bug, test failure, unexpected behavior | This file (below) |
| Need a reproduction strategy | `references/feedback-loops.md` |
| "flaky", "flake", "intermittent", "nondeterministic test" | `references/flaky-test-investigation.md` |
| "triage", "incident", "postmortem", "production down" | `references/triage.md` |
| First argument is a domain name (stripe, quality, etc.) | `references/audit.md` |
| "fix" | `references/fix.md` |
| "log issues" | `references/log-issues.md` |

No match above → this is a debugging session, continue below.

**The user's symptoms:** $ARGUMENTS

## Learnings

Before root-cause ranking, grep known failures:
`rg -n --glob '*.md' '^(title|tags|applies_when):|<module>|<symptom>|<failure-mode>' docs/solutions`.
Open likely matches and test any applicable known cause first.

## The Iron Law

```
NO FIXES WITHOUT A FEEDBACK LOOP AND ROOT CAUSE FIRST
```

Build the smallest believable pass/fail loop before hypothesizing deeply — the
general verification contract lives in
`primitives/shared/references/verification-system-first.md`; diagnose's own
loop menu, loop-quality checklist, correct-seam rule, and the instrumented
human-reproduction protocol live in `references/feedback-loops.md`. If no
loop is possible with current access, stop and ask for a captured artifact,
environment access, or temporary instrumentation.

## Rule #1: Config Before Code

External service issues are usually config, not code. Check in order:

1. **Env vars present?** `npx convex env list --prod | grep <SERVICE>` or `vercel env ls`
2. **Env vars valid?** No trailing whitespace, correct format
3. **Endpoints reachable?** `curl -I -X POST <webhook_url>`
4. **Then** examine code

If the symptom is external-service behavior and the provider surface is
supported by `emulate.dev`, build the feedback loop against the local emulator
before blaming production config or writing code. Do not use emulation to prove
provider-only concerns such as real auth, billing, quota, or live webhook
delivery. Usage details: https://emulate.dev/docs.

## Root Cause Discipline

For each hypothesis, categorize:
- **ROOT:** fixing this removes the fundamental cause.
- **SYMPTOM:** fixing this masks an underlying issue.

Post-fix question: "If we revert in 6 months, does the problem return?" Rank
3-5 hypotheses when the cause isn't obvious and test one prediction at a time
— the full scientific-method protocol, pattern-analysis technique, and the
3+-failed-fixes architecture check live in
`references/systematic-debugging.md`.

## Classification

| Type | Signals | Approach |
|------|---------|----------|
| Test failure | Assertion error | Read test, trace expectation |
| Runtime error | Exception, crash | Stack trace -> source -> state |
| Type error | TS complaint | Read error, check types |
| Build failure | Bundler error | Check deps, config |
| Behavior mismatch | "Does Y, should do X" | Trace code path |
| Performance | Slow, timeout | Add timing instrumentation |
| Production incident | Incident tracker, alerts | Route to `references/triage.md` |

## Red Flags — Stop and Rebuild the Loop

- "Quick fix for now, investigate later"
- "Just try changing X and see"
- Multiple simultaneous changes
- Proposing solutions before tracing data flow
- "One more fix attempt" (when 2+ already tried) — see the architecture check
  in `references/systematic-debugging.md`
- Each fix reveals a new problem in a different place

## Completion Gate

See `primitives/shared/AGENTS.md` (Completion Evidence) for the shared core;
this phase adds:
- **Root cause:** what's actually wrong, classified ROOT vs SYMPTOM.
- **Fix:** how it was resolved — one change at a time, then the original
  loop rerun (not just the minimized regression).
- **Verification:** a log entry, metric, or database state that proves the
  fix worked — not "the code looks right now". Mark **UNVERIFIED** until an
  observable confirms it. Semantic waiver: diagnosis quality ties to the
  concrete failure and cannot be proven by a static fixture — cite
  reproduction evidence, the hypothesis test, and the exact command or
  runtime surface exercised.
- **Learning:** if the fix revealed a reusable repo-technical pattern, offer
  `/compound` while evidence is fresh.
