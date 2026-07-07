---
name: deliver
description: |
  Take one ticket or idea from raw intent to merge-ready (or shipped, when
  asked): context-first, docs→tests→code, live QA, refactor at three
  altitudes, semantic commits, diverse-provider review, adversarial pre-ship
  thinking. Use for "deliver this", "build this ticket", "make it
  merge-ready", "take this end to end". Trigger: /deliver.
argument-hint: "[backlog-item|description]"
---

# /deliver

One piece of work, end to end, done the way we'd be proud of. This skill is
judgment checkpoints, not phase machinery — you own the decomposition, and
even the best model skips some of these steps unprompted. Don't.

## Context first

Before writing anything: read the ticket and the live code it touches; build
real product understanding (who uses this, what breaks if it's wrong);
`/research` best practices when the design has open questions; and
**pre-factor** — if the change lands in messy ground, clean the ground first
as its own commit. If the ticket has no goal or acceptance oracle, run
`/shape` (or write the oracle yourself for small work) before building.
For product direction, positioning, long-lived workflow, or project-identity
changes: read root VISION.md when present; if missing or stale, route to
`/vision` before building.
If the goal is still in the user's head, interrogate before shaping with the
interrogate-first lens (`primitives/shared/references/interrogate-first.md`); one
good question beats a guessed plan.
Building without an oracle yields plausible garbage. For public API, CLI, UI,
performance, compatibility, migration, or operator workflow changes, load
`primitives/shared/references/works-critique.md` before pre-ship review. Before
adding surface for automation, optimization, or refactor pressure, see
`primitives/shared/references/delete-first.md` (Ponytail:
`primitives/skills/.external/dietrich-ponytail/SKILL.md`).

For non-trivial execution, start from the HTML plan artifact when one exists;
otherwise author a small one directly from
`primitives/skills/shape/templates/html-plan.html`, open it, and use the rendered page to
check clarity, sequence, risk, and proof before coding. The plan must stand
alone for the executor: hero as the work contract, support sections for
alternatives/tradeoffs, acceptance, verification, communication cadence, stop
conditions, and useful adversarial review. Include the quality system from
`primitives/shared/references/quality-system.md`: standards, proof methods,
critic topology, and stop rules. The point is visual planning, not a Markdown
conversion. Skip only for trivial mechanical fixes, no-browser environments, or
explicit operator waiver.

**Verification system first** (shared AGENTS.md, Layer 1): locate the
repo's live-verification harness — the one command that exercises a change
against the running thing and emits reviewable evidence. If none exists,
building it is the ticket's first milestone, not a detour; every milestone
after ships through it. The harness-before-feature session catches the
bugs unit tests structurally cannot. For evals, benchmarks, QA paths, agent
behavior, performance, or unclear proof loops, load
`primitives/shared/references/verification-system-first.md` and name the
claim, falsifier, driver, grader, evidence packet, and cadence before edits.

## Docs → tests → code

Write the documentation first: what will be true when this works (README
section, doc comment, API doc — whatever the repo's convention is). Then
failing tests that encode it. Then code to green. After green, loop back and
sync all three — docs, tests, and code must agree at the end, not just the
start. Work on a feature branch; never commit to the default branch.

Lenses, when judgment is contested: Ousterhout (deep modules, small
interfaces), Carmack (shippability — what can be cut), Kent C. Dodds (test
what users do, not implementation), Uncle Bob (leave it cleaner than found).

## Deviations

The plan will be wrong somewhere — live code always holds unknowns the shape
didn't map. When an edge case forces an off-plan choice that doesn't break
the shape itself: pick the conservative option, log it in a **Deviations**
section of the plan artifact (site, what forced it, what you chose), and
keep going. If the deviation invalidates the shape, that's the re-shaping
stop (Gotchas), not a ledger entry.

The ledger is routed, not private notes: reviewers get the deviation sites
as risk coordinates (locations, never your justifications — that stays the
reasoning-trail rule), `/qa` drives them as the edges most plausibly broken,
and each entry is a discovered unknown that feeds the shape or backlog after
landing.

## QA the live thing

`/qa` routes by app shape (browser, API, CLI, library, MCP) and owns the
"tests pass is not verification" claim. Run it through the verification
harness you located or built in Context first. Leave an evidence packet —
screenshots, transcripts, request/response pairs, a verdict — where the repo
keeps its receipts, so the claim is checkable after you're gone.

## Refactor at three altitudes

With working code in hand, ask at each level — and act on what you find. When
the refactor must prove it changed *nothing observable* and the target has no
characterization tests, "unit tests pass" cannot see the seam a lift most
often breaks — reach for the live-diff pattern in
`primitives/shared/references/verification-system-first.md` (diff the local
branch against the deployed/pre-refactor build over the same backing store).

1. **The diff.** Would we write it this way from scratch? Duplicate code to
   DRY out? Conditionals to collapse? Slop to delete (unnecessary comments,
   defensive try/catch, casts, dead branches)?
2. **The codebase.** Is there a cleaner seam? Does this change reveal an
   abstraction that should exist — or one that should die?
3. **The backlog.** Does what you learned change what the product should do
   next? File tickets for the bigger moves; don't smuggle them into this
   branch.

For non-trivial code, add the synced
`thermo-nuclear-code-quality-review` skill
(`primitives/skills/.external/cursor-thermo-nuclear-code-quality-review/SKILL.md`) as
the harsh maintainability pass before declaring the diff clean. julius-caveman
for interim synthesis only; findings, code, commits, and final artifacts stay
normal English.

## Land it

- **Semantic conventional commits**, push (the `/yeet` discipline: classify,
  split by concern, why-shaped bodies).
- **Review by diverse providers — never only yourself.** `/code-review` fans
  out fresh-context reviewers across model families; fix blockers, re-review
  until clean. Iterate until CI is green *and rigorous* — if the gate
  wouldn't have caught the likely failure, strengthening it is part of this
  ticket (`/ci`). Respond to every review finding: fix, ticket, or rejected
  with a stated reason.
- **Think adversarially before shipping.** If this breaks in production,
  what breaks first? How would we know — which log line, metric, or alert?
  If the answer is "we wouldn't," add the logging/alerting now, not after
  the incident. Dependency upgrades ride as one curated, risk-assessed
  commit — never a pile of unexamined bumps.
- **Default stop: merge-ready.** Squash-merge to master (the `/ship`
  trailer canon, `meta/CONTRACTS.md`) only when the operator asked for
  shipped, not just delivered. Then monitor if the repo has a post-ship
  signal, and ta-da.

## Completion Gate

See `primitives/shared/AGENTS.md` (Completion Evidence) for the shared core.
`/deliver` adds:

- Deviation ledger (or "none").
- Final `git status --short` + branch/remote state — a clean tree is part of
  done; every leftover path gets committed, ignored, moved out, or named as a
  blocker.
- Offer `/compound` before the evidence goes stale if the work produced a
  reusable repo-technical lesson.

## Gotchas

- **Skipping shape.** No oracle → no delivery. Write one or route to
  `/shape`.
- **Re-shaping mid-build.** If implementation reveals the shape is wrong,
  stop and say so — don't spin on a broken spec.
- **Self-review leniency.** The author's context rationalizes the author's
  choices. Reviewers get the diff and the oracle, never your reasoning
  trail.
- **Stale tickets.** If the item already shipped (closure trailer in
  history, `_done/` copy), refuse and fix the backlog instead.
- **Heavy/parallel lanes** route to `/sprites`; quick exploration stays
  local. Don't pre-shred work into atomic tasks — an outcome-shaped lane
  owns its own decomposition.
