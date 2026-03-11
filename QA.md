# Bitterblossom QA Runbook

This runbook covers the current Bitterblossom surface:

- `bb` is the thin sprite transport CLI
- `scripts/conductor.py` is the run-centric control plane

Legacy shell wrapper commands such as `bb provision`, `bb sync`, `bb teardown`, and `bb watchdog` are not part of the supported CLI.

## Build and Regression Checks

```bash
make build
make test
make test-python
make lint-python
```

For transport-only verification:

```bash
go test ./cmd/bb/...
```

## Required Environment

```bash
source .env.bb
export GITHUB_TOKEN="$(gh auth token)"
export SPRITE_TOKEN="..."         # preferred
# or export FLY_API_TOKEN="..."   # fallback
export OPENROUTER_API_KEY="..."   # required for bb setup
```

## Transport Smoke Test

1. Set up one sprite for a repo:

```bash
bb setup <sprite> --repo <owner/repo>
```

2. Verify readiness without starting work:

```bash
bb dispatch <sprite> "dry-run readiness probe" --repo <owner/repo> --dry-run
```

3. Run one short task:

```bash
bb dispatch <sprite> "Describe the current branch state in TASK_COMPLETE and stop." --repo <owner/repo>
```

4. Inspect recovery surfaces:

```bash
bb status
bb status <sprite>
bb logs <sprite> --lines 50
```

5. If a dispatch was interrupted or left the sprite busy, recover with:

```bash
bb kill <sprite>
```

## Conductor Smoke Test

Validate environment first:

```bash
make conductor-check
```

Run the focused governance regression slice:

```bash
python3 -m pytest -q scripts/test_conductor.py -k 'acceptance_trace_bullet_run or duplicate_trusted_findings or low_severity_nit or novel_high_severity'
```

Then run one bounded conductor cycle on a prepared environment:

```bash
python3 scripts/conductor.py run-once \
  --repo <owner/repo> \
  --issue <number> \
  --worker <builder-sprite> \
  --reviewer <reviewer-a> \
  --reviewer <reviewer-b> \
  --reviewer <reviewer-c>
```

Inspect the run:

```bash
python3 scripts/conductor.py show-runs --limit 5
python3 scripts/conductor.py show-run --run-id <run-id>
python3 scripts/conductor.py show-events --run-id <run-id>
```

## Manual Checklist

- `bb setup` uploads base config, persona, Ralph assets, and repo metadata.
- `bb dispatch --dry-run` fails fast on auth or readiness problems.
- `bb dispatch` streams output and exits `2` when the agent writes `BLOCKED.md`.
- `bb status` shows fleet reachability and single-sprite git/signal details.
- `bb logs` reads `ralph.log` and supports `--follow` and `--json`.
- `bb kill` clears stale Ralph or Claude processes so the next dispatch can start.
- `scripts/conductor.py` can acquire a run, dispatch the builder, collect review evidence, and surface run state through `show-runs` / `show-run` / `show-events`.
