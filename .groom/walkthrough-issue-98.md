# Walkthrough: Issue 98 Run Surfaces

## Merge Claim

The conductor now exposes run-centric inspection surfaces that show heartbeat recency and blocking context directly from the run/event ledger, plus a single-run JSON view with recent event context.

## Why Now

Issue #98 requires truthful operator visibility into active and blocked runs without dropping to raw SQLite queries. Before this branch, `show-runs` only emitted a thin subset of run fields and there was no single `run_id` command that returned both run metadata and recent events together.

## Before

- `python3 scripts/conductor.py show-runs --limit 20` emitted only `run_id`, issue metadata, phase/status, builder sprite, PR number, and `updated_at`.
- Operators had to infer heartbeat recency manually and then run a second command or inspect SQLite tables to understand why a run was blocked or failed.

## After

- `show-runs` now emits `heartbeat_at`, `heartbeat_age_seconds`, `pr_url`, `blocking_event_type`, and `blocking_reason` when the run has blocking or failure context.
- `show-run --run-id <id>` returns one JSON object with the full run surface and a `recent_events` array.

## Evidence

Fixture setup:

```bash
python3 scripts/conductor.py show-runs --db <tmp>/conductor.db --limit 1
```

Observed output:

```json
{"run_id":"run-98-demo","repo":"misty-step/bitterblossom","issue_number":98,"issue_title":"Observability","phase":"blocked","status":"blocked","builder_sprite":"fern","builder_profile":"claude-sonnet","branch":null,"pr_number":460,"pr_url":"https://github.com/misty-step/bitterblossom/pull/460","heartbeat_at":"2026-03-10T12:00:00Z","heartbeat_age_seconds":27216,"updated_at":"2026-03-10T19:33:28Z","blocking_event_type":"pr_feedback_blocked","blocking_event_at":"2026-03-10T19:33:28Z","blocking_reason":"PR review threads remained unresolved after revision"}
```

Single-run inspection:

```bash
python3 scripts/conductor.py show-run --db <tmp>/conductor.db --run-id run-98-demo --event-limit 2
```

Observed output:

```json
{"run":{"run_id":"run-98-demo","repo":"misty-step/bitterblossom","issue_number":98,"issue_title":"Observability","phase":"blocked","status":"blocked","builder_sprite":"fern","builder_profile":"claude-sonnet","branch":null,"pr_number":460,"pr_url":"https://github.com/misty-step/bitterblossom/pull/460","heartbeat_at":"2026-03-10T12:00:00Z","heartbeat_age_seconds":27216,"updated_at":"2026-03-10T19:33:28Z","blocking_event_type":"pr_feedback_blocked","blocking_event_at":"2026-03-10T19:33:28Z","blocking_reason":"PR review threads remained unresolved after revision"},"recent_events":[{"run_id":"run-98-demo","event_type":"pr_feedback_blocked","payload":{"reason":"unchanged_after_revision"},"created_at":"2026-03-10T19:33:28Z"},{"run_id":"run-98-demo","event_type":"ci_wait_complete","payload":{"passed":true},"created_at":"2026-03-10T19:33:28Z"}]}
```

## Persistent Verification

```bash
python3 -m pytest -q scripts/test_conductor.py
python3 -m ruff check scripts/conductor.py scripts/test_conductor.py
```

The regression coverage added in `scripts/test_conductor.py` verifies that:

- `show-runs` surfaces heartbeat age and blocking reason.
- `show-run` returns run metadata plus recent event context.
- persisted events like `ci_wait_complete` and `pr_feedback_blocked` render consistently through the operator surface.

## Residual Risk

- Heartbeat age is computed at read time, so values are intentionally time-relative and will vary between runs.
- The run surface intentionally summarizes the latest stop event instead of exposing a full semantic diagnosis tree in the top-level row.
