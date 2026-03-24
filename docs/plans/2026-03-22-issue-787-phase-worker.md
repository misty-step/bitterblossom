# Issue 787 Plan

## Problem

`Conductor.Fixer` and `Conductor.Polisher` duplicate the same polling, backoff,
task supervision, status, and completion machinery. That duplication already
creates review churn and makes multi-sprite-per-role impossible without copying
even more infrastructure.

## Product Spec

### Intent Contract

Replace the duplicated fixer/polisher runtime with one role-parameterized phase
worker that preserves the current single-sprite behavior and makes multiple
sprites per role a configuration change instead of another architectural fork.
The conductor should still decide when Thorn and Fern run, still expose truthful
status, and still recover workers when phase sprites come and go.

## Technical Design

- Add `Conductor.PhaseWorker` as the deep module that owns polling, candidate
  selection, per-sprite dispatch, in-flight tracking, task completion, backoff,
  health, and status for one role.
- Add `Conductor.PhaseWorker.Role` as the narrow behavior for role-specific
  work discovery and prompt construction.
- Move fixer-specific and polisher-specific judgment into
  `Conductor.PhaseWorker.Roles.Fixer` and
  `Conductor.PhaseWorker.Roles.Polisher`.
- Start one `PhaseWorker` per role with a list of healthy sprites instead of
  one named process per sprite module. Let health recovery update the role's
  sprite pool instead of starting a second bespoke worker tree.
- Keep the CLI/operator surface truthful by reporting per-role worker status
  from the new phase worker registry instead of hardcoded singleton module names.

## Acceptance Mapping

- Delete `Conductor.Fixer` and `Conductor.Polisher` without losing role-specific
  behavior.
- Preserve single-sprite fixer/polisher dispatch behavior.
- Support dispatching two eligible PRs in parallel when a role has two idle
  sprites.
- Preserve backoff, health, and status reporting semantics.
- Port focused fixer/polisher tests onto the shared worker contract.

## Slice

- Add a shaped issue body and this durable workpad before coding.
- Add failing tests for shared phase-worker behavior and multi-sprite dispatch.
- Implement `PhaseWorker`, role modules, and a small registry/supervision path.
- Rewire application boot, health monitor, and CLI status to the new worker.
- Delete the duplicated fixer/polisher modules and obsolete test expectations.

## Verification

- [x] Add RED tests for shared role-worker behavior, backoff, and two-sprite dispatch.
- [x] Implement the refactor behind the existing runtime contracts.
- [x] Run focused conductor tests for phase workers, health monitor, and boot.
- [x] Run the broader conductor suite after the focused slice is green.

Verified with:

```bash
cd conductor
mix compile --warnings-as-errors
mix test test/conductor/phase_worker_test.exs test/conductor/fleet/health_monitor_test.exs test/conductor/application_test.exs
mix test
```
