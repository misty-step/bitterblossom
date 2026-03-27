# Factory Audit: 2026-03-27 Run 2 — Post-Fix Supervised Run

## Run metadata
- Start: 2026-03-27T17:21:18Z
- End: 2026-03-27T17:26:09Z (5 min supervised)
- Fleet: 5 sprites (bb-builder, bb-fixer, bb-polisher, bb-polisher-2, bb-polisher-3)
- Target observation: Can the factory self-serve P1 issues autonomously?

## Boot metrics
- Reconciliation: 5/5 healthy (including bb-fixer — **probe fix confirmed working**)
- Spellbook bootstrap: 5/5 complete in ~5s
- All agents dispatched by T+8s
- bb-polisher-2 crashed at T+28s, **auto-restarted at T+58s** (restart fix confirmed working)

## Agent activity during 5-minute window

| Agent | Action | Judgment quality |
|-------|--------|-----------------|
| Weaver | Claimed #778 (sprite lifecycle), explored sprite CLI API for checkpoint management | Good — chose actionable P1, researched before coding |
| Thorn | Closing stale PRs targeting deleted conductor modules | Correct — following AGENTS.md close-stale-PRs directive |
| Fern #1 | Deep analysis of PR #764 vs master, determined it targets deleted surfaces | Correct — thorough before closing |
| Fern #2 | Found no merge-ready PRs, idle | Correct — nothing to do |
| Fern #3 | Closed PR #764 with explanation | Correct — good judgment |

## Findings

### F1: bb-polisher-2 crash + restart [VERIFIED WORKING]
The agent crashed ("harness does not support continuation"), was detected, and restarted after 30s backoff with fresh spellbook bootstrap. This is the #801 fix in action.

### F2: All 5 sprites healthy including bb-fixer [VERIFIED WORKING]
bb-fixer woke via websocket in ~7s (was previously timing out at 15s on http-post). This is the #804 fix in action.

### F3: Weaver picked #778 instead of #800
Not a bug — both are P1/next. Weaver exercised judgment. However, #800 (dead code purge) is the more impactful issue. Consider whether the AGENTS.md should bias toward effort/s items first (quick wins) vs effort/m (bigger lifts).

### F4: Fern correctly closed stale PR #764
Autonomous judgment: analyzed the PR diff against master, determined it modifies deleted surfaces, closed with explanation. Exactly the behavior defined in the AGENTS.md.

### F5: "harness does not support continuation" crash on bb-polisher-2
The codex harness exits non-zero when it can't continue a session. Root cause unclear — could be: workspace state, auth, or a codex bug. The restart loop handles it, but the underlying cause should be investigated to prevent wasted cycles.

## Scores (delta from previous audit)

| Area | Previous | Now | Delta |
|------|----------|-----|-------|
| Agent autonomy | 9/10 | 9/10 | — |
| Infrastructure simplicity | 6/10 | 7/10 | +1 (code_host deleted, restart added) |
| Test health | 8/10 | 8/10 | — |
| Observability | 5/10 | 5/10 | — |
| Resilience | 4/10 | 7/10 | +3 (probe fix, auto-restart) |
| Multi-repo readiness | 3/10 | 3/10 | — (Weaver claimed #803 but not yet shipped) |

## Issues filed/updated
- #804: CLOSED (probe fix merged in #805)
- #801: CLOSED (restart fix merged in #806)
- #764: CLOSED by Fern agent autonomously

## Conclusion
The factory is self-healing and agents exercise correct judgment. Two infrastructure fixes are confirmed working in production. The main gap is multi-repo support (#803) and dead code cleanup (#800) — both in the backlog, both P1.
