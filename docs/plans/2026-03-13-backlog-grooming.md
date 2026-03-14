# 2026-03-13 Backlog Grooming

## Scope

Backlog pass for Bitterblossom using:
- `groom`
- `agent-backlog`
- `context-engineering`
- `llm-infrastructure`
- `harness-engineering`

## Backlog Health

- Open issues: 46
- Unlabeled open issues: 11
- Open issues without milestones: 23
- Open issues missing the canonical label set: 18
- `project.md` active focus was stale relative to the live backlog

## Diagnosis

The backlog is not directionless, but it is only partly normalized. The core plan is visible in `#569`, `#500`, `#590`, `#592`, and `#593`, while a long tail of unlabeled or weakly-scoped follow-ups still sits beside them as if they were first-class roadmap items.

The context layer has drifted from the roadmap. `project.md` still pointed at older focus issues even though the active architecture shift has moved to workflow contract, governance truth surfaces, and phase-specialized workers.

The harness layer is decent for code feedback and telemetry, but weak for agent-planning feedback. Prompt/model/runtime choices are observable, yet the exact model identifiers and runtime contract are not mechanically verified against current provider docs.

## Canonical Roadmap

Keep the active roadmap centered on these issues:
- `#569` umbrella for the factory simplification
- `#500` semantic finding ledger and convergence
- `#590` single repo-owned workflow contract
- `#592` phase-specialized worker packs
- `#593` separate semantic, policy, and mechanical merge truth
- `#544` incident-aware replay and false-red handling
- `#532` sprite CLI auth interoperability

These are the issues that currently define the system shape. The backlog should read from them outward, not sideways around them.

## Reduction Recommendations

### Normalize immediately

- Add canonical labels and milestones to unlabeled open issues: `#553`, `#556`, `#566`, `#604`, `#508`, `#537`, `#507`, `#554`, `#498`, `#565`, `#564`
- Assign `Next: Up Next` to open `next` issues missing milestones, especially `#569`, `#590`, `#592`, `#593`, `#541`, `#542`, `#493`, `#506`, and `#584`

### Merge or rewrite

- Fold `#565` into `#541` unless it grows into a deeper architectural boundary than “one policy path”
- Fold `#564` into `#541` or the next module-boundary test issue; it is too shallow alone
- Rewrite `#553` into canonical issue format; it is a real blocker, but not currently execution-ready enough
- Rewrite `#508` into canonical issue format; the intent is good but it needs labels, milestone, and touchpoints
- Rewrite `#604` into canonical issue format; it is a correctness guardrail, not just an architecture note

### Long-tail cleanup

- Re-evaluate `#523`, `#524`, `#525`, `#527`, `#528`, and `#530` as a batch
- Keep only the items that still buy leverage after `#500` and `#501`; merge or close the rest as proof-gap nits
- Re-home docs-only follow-ups like `#522` under `#488` if they do not stand alone as roadmap work

## Missing Strategic Work

One important gap was not represented cleanly in the open backlog at the start of this pass:

- **LLM runtime contract verification**: Bitterblossom records semantic traces, but it does not yet enforce a single source of truth for model IDs, provider-specific runtime settings, or model-currency checks against current primary docs

That gap is now tracked as `#606`.

## Recommended Next Slice

1. Finish `#500` so review truth is durable.
2. Land `#590`, `#592`, and `#593` as the explicit contract layer around that truth.
3. Fix `#532` and `#553` so the factory’s auth/bootstrap path is trustworthy again.
4. Add the missing LLM runtime verification issue before more profile/model drift accumulates.

## Success Criteria For The Next Groom Pass

- Fewer than 10 open issues without full canonical labels
- Zero `next` issues without milestones
- `project.md` and the live roadmap name the same strategic work
- Every open `p1` issue is executable in one coherent pass without hidden context
