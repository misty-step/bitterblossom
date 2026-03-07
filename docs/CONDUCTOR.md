# Conductor

`scripts/conductor.py` is the Bitterblossom conductor MVP.

It is not another transport CLI. It owns:

- GitHub issue intake
- issue leasing
- builder dispatch
- reviewer council dispatch
- CI wait
- PR feedback / thread reconciliation
- merge
- run/event persistence

## Runtime Contract

State lives locally in:

- `.bb/conductor.db`
- `.bb/events.jsonl`

Remote run artifacts live on the worker sprite under:

- `${WORKSPACE}/.bb/conductor/<run_id>/builder-result.json`
- `${WORKSPACE}/.bb/conductor/<run_id>/review-<sprite>.json`

Before builder or reviewer dispatch, the conductor probes sprite readiness with
`bb dispatch --dry-run`. Builder selection is probe-only: unhealthy workers are
skipped immediately so the conductor can fall through the pool quickly. Reviewer
readiness is stricter: if a probe fails, the conductor attempts one forced
repair with `bb setup <sprite> --repo <owner/repo> --force`, then re-probes.
Runs fail fast before builder work if the reviewer pool cannot be made
dispatch-ready.

## Environment

Required:

- `GITHUB_TOKEN`
- `SPRITE_TOKEN` or `FLY_API_TOKEN`

Typical local shell:

```bash
source .env.bb
export GITHUB_TOKEN="$(gh auth token)"
```

## Makefile Targets

```bash
make test-python      # python3 -m pytest -q base/hooks scripts/test_conductor.py
make lint-python      # ruff check base/hooks scripts/conductor.py scripts/test_conductor.py
make conductor-check  # validate coordinator runtime environment
```

## Commands

Validate coordinator environment before starting:

```bash
python3 scripts/conductor.py check-env
```

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

`--trusted-external-surface` is exact, not substring-based. Use exact status
context names such as `CodeRabbit` / `Greptile Review`, or an exact workflow
name such as `Cerberus` when you want to wait on a whole check-run family.

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

## Remote Deployment

The conductor is designed to run on a dedicated coordinator sprite, not a developer laptop. Laptops sleep, shells drift, and tokens expire silently. The coordinator is always-on.

### Bootstrap

1. Create the coordinator sprite (one-time):

```bash
sprite create coordinator
```

2. Push the repo and toolchain:

```bash
bb setup coordinator --repo misty-step/bitterblossom
```

3. Set required secrets on the coordinator:

```bash
sprite exec coordinator -- bash -lc '
  echo "export GITHUB_TOKEN=..." >> ~/.bashrc
  echo "export SPRITE_TOKEN=..." >> ~/.bashrc
'
```

4. Validate the environment:

```bash
sprite exec coordinator -- bash -lc '
  cd /home/sprite/workspace/bitterblossom
  make build
  python3 scripts/conductor.py check-env
'
```

All checks must pass before starting the loop.

### Starting the Loop

Run the conductor in the background on the coordinator. Use `nohup` so it survives session disconnects:

```bash
sprite exec coordinator -- bash -lc '
  cd /home/sprite/workspace/bitterblossom
  nohup python3 scripts/conductor.py loop \
    --repo misty-step/bitterblossom \
    --label autopilot \
    --worker noble-blue-serpent \
    --reviewer council-fern-20260306 \
    --reviewer council-sage-20260306 \
    --reviewer council-thorn-20260306 \
    >> ~/.bb/conductor.log 2>&1 &
  echo "conductor pid: $!"
'
```

The loop polls for eligible issues every 60 seconds (configurable with `--poll-seconds`). Transient failures log and continue; blocked runs (requiring human review) are noted in the issue and the loop moves on.

### Verifying the Loop

Check run state:

```bash
sprite exec coordinator -- bash -lc '
  cd /home/sprite/workspace/bitterblossom
  python3 scripts/conductor.py show-runs --limit 5
'
```

Tail conductor logs:

```bash
sprite exec coordinator -- bash -lc 'tail -f ~/.bb/conductor.log'
```

### Durable Run State

Every run writes immediately to `.bb/conductor.db` and `.bb/events.jsonl` on the coordinator. State survives loop restarts. If the conductor process dies, restart it — already-completed runs won't be re-processed because their leases have been released.

## Blocked Runs

A run exits with `rc=2` (blocked) when the conductor cannot proceed without human input — examples:

- reviewer council blocked after max revision rounds
- an untrusted PR review thread requires maintainer review
- PR review threads remain unresolved after a revision pass

When a run is blocked the conductor **does not release the issue's lease**. Instead it marks the lease as blocked (`blocked_at` in the leases table and `lease_expires_at = null`). The blocked issue is excluded from backlog selection on all subsequent polls — it will not be re-tried automatically.

### Identifying blocked issues

```bash
python3 scripts/conductor.py show-runs --limit 20
```

Blocked runs show `phase=blocked` and `status=blocked`. The associated issue also has a GitHub comment from Bitterblossom explaining why it was blocked.

### Re-queuing a blocked issue

After reviewing the blocking reason and making any necessary adjustments (e.g., resolving the PR thread manually, updating the issue body), re-queue the issue:

```bash
python3 scripts/conductor.py requeue-issue \
  --repo misty-step/bitterblossom \
  --issue-number <N>
```

This clears the blocked state and releases the lease. The issue becomes eligible on the next backlog poll.

To inspect the blocked run's events before re-queuing:

```bash
python3 scripts/conductor.py show-events --run-id <run-id>
```

## Operator Recovery

### Loop Died

Check why:

```bash
sprite exec coordinator -- bash -lc 'tail -50 ~/.bb/conductor.log'
```

Fix the root cause, then restart the loop as documented above.

### Stuck or Stale Issue

If a run is stuck (sprite unresponsive, lease expired), the conductor reclaims expired leases automatically on the next poll cycle. To force-release a lease manually:

```bash
sprite connect coordinator
cd /home/sprite/workspace/bitterblossom
sqlite3 .bb/conductor.db \
  "update leases set released_at = datetime('now') where issue_number = <N> and released_at is null"
```

### Worker Sprite Stuck

Kill the stuck ralph loop and let the conductor retry:

```bash
bb kill noble-blue-serpent
```

### Reconcile After Manual Merge

If a PR was merged out-of-band, sync the run store:

```bash
python3 scripts/conductor.py reconcile-run --run-id <run-id>
```

## Merge Policy

The target repo currently requires a `merge-gate` status on `master`.

This repo now publishes `merge-gate` in GitHub Actions. The conductor also checks for missing required statuses before it attempts merge, so policy mismatches fail loudly instead of pretending CI is complete.

This repo also requires resolved PR conversations. After CI turns green, the conductor queries unresolved review threads, routes that feedback back to the builder on the existing PR, and only proceeds once the conversation gate is clear. If the same threads still block after a revision pass, the conductor stops with `pr_feedback_blocked` and escalates to a human for confirmation.

## Review Council

Reviewer dispatches now run in parallel, not serially. Each reviewer writes its own artifact, the conductor persists that result immediately, and `review_complete` events land as each artifact arrives. One slow reviewer no longer hides the rest of the council's progress.

## Prompt Input Trust Model

The conductor constructs prompts from several sources. Not all of them are trusted.

| Input | Source | Trusted? | Handling |
|---|---|---|---|
| `issue.title`, `issue.body` | GitHub Issues (public, user-controlled) | **No** | JSON-fenced + untrusted-data header via `wrap_untrusted_issue_content` |
| PR review thread comments (`source=pr_review_threads`) | External GitHub reviewers / bots | **No** | JSON-fenced + untrusted-data header via `format_builder_feedback` |
| Internal sprite review summary (`source=review`) | Trusted conductor-owned sprites | Yes | Embedded plain-text |
| Run metadata (run ID, branch, artifact path) | Conductor internals | Yes | Embedded plain-text |

**Rule:** any string that originates outside the conductor (GitHub, external review bots) is wrapped in a JSON code-fence before being placed in a prompt. The wrapper includes an explicit instruction telling the agent to treat the block as data, not as executable guidance.

The `wrap_untrusted_issue_content` helper (conductor.py) implements this for issue content. `format_builder_feedback` implements the same pattern for PR review thread feedback.

## MVP Limits

- one builder per issue
- one review council round loop
- one PR-feedback revision loop
- SQLite only
- single-tenant worker assumption
- deterministic issue filtering, heuristic ranking for now
- shared repo checkout on workers for now

The accepted next cuts are in [ADR-003](./adr/003-conductor-control-plane.md): lease heartbeats, stale-lease reclaim, resume-first reconciliation, semantic routing, per-run worktrees, and parallel variants.
