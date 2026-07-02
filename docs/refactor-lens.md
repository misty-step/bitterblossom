# Refactor Lens Decision

Date: 2026-07-02
Backlog: 071

## Decision

Refactor remains a read-only review lens, not a standalone dispatch workload.
The canonical implementation is the existing `simplification` verdict member in
the submission storm.

The lens may report avoidable complexity, dead code, needless abstraction,
duplicate logic, and gate weakening. It does not edit code, push branches, open
PRs, merge, comment, or create issues.

## Why

The event plane already has two authoring paths:

- operator-initiated structural work through `bb run build` or a direct operator
  session;
- review-time detection through the `simplification` storm member.

Adding a `refactor` dispatch task would create a second mutating authoring path
without a new safety property. The safer surface is: detect structural risk in
review, then route any desired diff through `build` or the operator. That diff
becomes an ordinary branch or PR and re-enters the verdict storm.

## Boundary

`simplification` is the structural review lens. A future deeper structural lens
may replace or extend that card, but it remains read-only unless a new ticket
proves why a diff-producer is necessary.

If a future diff-producer is accepted, it must keep these invariants:

- no auto-merge;
- output is a branch/PR that re-enters the submission storm;
- loop termination remains the gate's `max_rounds`;
- no workload-specific Rust code.

Until then, do not add `plane/tasks/refactor`, an example refactor template, or
special dispatch logic for refactoring.
