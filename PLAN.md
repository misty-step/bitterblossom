# PLAN: Issue #544 — Incident-aware replay and false-red waiver handling

## Problem

RunServer collapses all failures into generic blocked/failed states. Known false-reds on
trusted external surfaces reopen code work unnecessarily. No incident audit trail exists.

## Solution Slice

Add a `Conductor.Recovery` module that classifies failures and drives policy decisions.
Extend Store with `incidents` and `waivers` tables. Update RunServer CI evaluation to
use recovery-aware logic. Add CLI inspection commands.

## Failure Classes

- `:transient_infra`      — network, timeout, infra noise
- `:auth_config`          — credential or config misconfiguration
- `:semantic_code`        — actual code/test failure
- `:flaky_check`          — known intermittent check
- `:known_false_red`      — external trusted surface failure (e.g. Cerberus)
- `:human_policy_block`   — requires human approval
- `:unknown`              — unclassified

## Key Invariants

- Semantic readiness is independent of mechanical check state
- Waiver recording is durable (events + waivers table)
- Replay is bounded (max_replays config, default 3)
- Merge via CLI only; no auto-merge of true code failures

## Files to Create

- `conductor/lib/conductor/recovery.ex`
- `conductor/test/conductor/recovery_test.exs`

## Files to Modify

- `conductor/lib/conductor/store.ex` — incidents/waivers tables, new CRUD, new run columns
- `conductor/lib/conductor/run_server.ex` — CI evaluation via Recovery, replay lane
- `conductor/lib/conductor/cli.ex` — show-incidents, show-waivers commands
- `conductor/lib/conductor/config.ex` — max_replays, replay_delay_seconds config

## Steps

- [x] Read context (MEMORY.md, WORKFLOW.md, existing modules)
- [ ] Implement Conductor.Recovery (classify_check, evaluate_with_policy)
- [ ] Extend Store (tables, CRUD, update_run columns)
- [ ] Update RunServer (recovery-aware check_ci, replay lane)
- [ ] Add CLI commands (show-incidents, show-waivers)
- [ ] Write tests (recovery_test.exs, extend store/github tests)
- [ ] Run `mix test` and fix failures
- [ ] Create PR

## Review Notes

(fill in after implementation)
