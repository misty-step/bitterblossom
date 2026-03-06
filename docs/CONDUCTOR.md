# Conductor

`scripts/conductor.py` is the Bitterblossom conductor MVP.

It is not another transport CLI. It owns:

- GitHub issue intake
- issue leasing
- builder dispatch
- reviewer council dispatch
- CI wait
- merge
- run/event persistence

## Runtime Contract

State lives locally in:

- `.bb/conductor.db`
- `.bb/events.jsonl`

Remote run artifacts live on the worker sprite under:

- `${WORKSPACE}/.bb/conductor/<run_id>/builder-result.json`
- `${WORKSPACE}/.bb/conductor/<run_id>/review-<sprite>.json`

## Environment

Required:

- `GITHUB_TOKEN`
- `SPRITE_TOKEN` or `FLY_API_TOKEN`

Typical local shell:

```bash
source .env.bb
export GITHUB_TOKEN="$(gh auth token)"
```

## Commands

Run one issue:

```bash
python3 scripts/conductor.py run-once \
  --repo misty-step/bitterblossom \
  --issue 450 \
  --worker noble-blue-serpent \
  --reviewer council-fern-20260306 \
  --reviewer council-sage-20260306 \
  --reviewer council-thorn-20260306
```

Run continuously against the backlog:

```bash
python3 scripts/conductor.py loop \
  --repo misty-step/bitterblossom \
  --label autopilot \
  --worker noble-blue-serpent \
  --reviewer council-fern-20260306 \
  --reviewer council-sage-20260306 \
  --reviewer council-thorn-20260306
```

Inspect runs:

```bash
python3 scripts/conductor.py show-runs --limit 20
python3 scripts/conductor.py show-events --run-id run-450-1772813415
```

Reconcile a run after out-of-band merge or manual recovery:

```bash
python3 scripts/conductor.py reconcile-run --run-id run-450-1772813415
```

## Merge Policy

The target repo currently requires a `merge-gate` status on `master`.

This repo now publishes `merge-gate` in GitHub Actions. The conductor also checks for missing required statuses before it attempts merge, so policy mismatches fail loudly instead of pretending CI is complete.

## MVP Limits

- one builder per issue
- one review council round loop
- SQLite only
- single-tenant worker assumption
- deterministic issue filtering, heuristic ranking for now
- shared repo checkout on workers for now

The accepted next cuts are in [ADR-003](./adr/003-conductor-control-plane.md): lease heartbeats, stale-lease reclaim, resume-first reconciliation, semantic routing, per-run worktrees, and parallel variants.
