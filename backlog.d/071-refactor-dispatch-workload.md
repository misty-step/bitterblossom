# (Deferred) Refactor as a focused PR-review subagent, not a dispatch workload

Priority: P3 | Status: pending | Estimate: M

## Goal

Deferred (2026-06-17): revisit refactor after **dispatch flows + code review**
are fully working. Leading hypothesis: refactor is **not** a standalone dispatch
workload but a **focused PR-review subagent/lens** — refactor opportunities
surface as read-only review findings (the existing `simplification` storm member
is the seed), and the diff, when wanted, comes from the existing `build` dispatch
path or the operator — not from a new auto-mutating workload.

## Why deferred

Focus decision: get the two flows we actually run — operator **dispatch**
(`build`) and the **code-review** reflex storm — robust, recoverable, and
observable before adding workload types. The refactor design also had unmade
decisions (dispatch-as-safety vs reflex, the review→refactor loop terminator,
task-vs-card) that don't need resolving until the foundation is solid.

## When we revisit, decide

- [ ] Lens or diff-producer? A read-only review lens (detection only; diff via
      `build`/operator) is cheaper, reuses `simplification`, and has no
      auto-mutation risk. Default to the lens unless a diff-producer earns it.
- [ ] If a lens: how does it differ from `simplification` (deeper structural
      proposals?), and does it just enrich the review report?
- [ ] If ever a diff-producer: the no-auto-merge invariant (output is a PR that
      re-enters review) and the review→refactor loop terminator (storm
      `max_rounds`).

## Notes

Original framing (a dispatch workload sibling of `build`) is parked, not deleted.
Today refactor exists only as the read-only `simplification` lens
(`plane/tasks/simplification/card.md`); the operator's `/refactor` discipline and
the `build` workload already cover operator-initiated structural work. Superseding
focus: dispatch + code review — see 051, 064, 066, 068, 069, 070, 072.
