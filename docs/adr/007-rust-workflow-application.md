# ADR 007: Rust workflow application and deterministic composition

Status: accepted migration contract, 2026-07-21

## Decision

Workflow revisions store one canonical desired agent composition. Activation validates
that its controls are enforceable, materializes an immutable launch contract, and
persists a digest for each step. Runs and evidence use that accepted snapshot, so a
later Roster/catalog change cannot alter an accepted launch. Fallbacks are ordered,
deterministic, and narrowing: they cannot widen role, tools, context, authority,
seats, or run-group budget.

Bitterblossom has three independent agentic loops: composition, execution, and
verification/remediation. The final merge gate is a deterministic controller-owned
exact-head actuator. It re-reads trusted PR/head/check/review/risk/approval facts,
invokes the typed merge effect with the exact SHA, and completes Powder from the
receipt. Independent reviewer judgment remains upstream.

A fourth declaration may remain temporarily as migration syntax, but it is not a
fourth agent workflow and must not be commissioned as model judgment.

## Consequences

The database is authoritative for accepted workflow state; TOML and JSON remain
interchange forms. Snapshot digests and readback evidence make activation and
execution auditable. Unsupported or unenforceable controls fail activation or
pre-execution rather than silently becoming advisory.
