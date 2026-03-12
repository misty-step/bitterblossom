# Conductor

`scripts/conductor.py` is the Bitterblossom conductor MVP.

It is not another transport CLI. It owns:

- GitHub issue intake
- issue leasing
- builder lane dispatch
- governor lane adoption
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
`bb dispatch --dry-run`. Builder workers are now modeled as logical slots:
`--worker fern:2 --worker sage` means two builder slots on `fern` and one on
`sage`. Unhealthy builder slots accrue probe failures in SQLite and drain
themselves after repeated failures so the conductor falls through to healthy
capacity instead of retrying the same broken slot immediately. Reviewer
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

Stop after PR handoff and let governance pick the PR up later:

```bash
python3 scripts/conductor.py run-once \
  --repo misty-step/bitterblossom \
  --issue 479 \
  --worker noble-blue-serpent \
  --reviewer council-fern-20260306 \
  --reviewer council-sage-20260306 \
  --reviewer council-thorn-20260306 \
  --stop-after-pr
```

Adopt a known PR into the governor lane:

```bash
python3 scripts/conductor.py govern-pr \
  --repo misty-step/bitterblossom \
  --issue 479 \
  --pr-number 490 \
  --worker noble-blue-serpent \
  --reviewer council-fern-20260306 \
  --reviewer council-sage-20260306 \
  --reviewer council-thorn-20260306
```

`--trusted-external-surface` is exact, not substring-based. Use exact status
context names such as `CodeRabbit` / `Greptile Review`, or an exact workflow
name such as `Cerberus` when you want to wait on a whole check-run family.

`--pr-minimum-age-seconds` delays governance until a PR is old enough for async
review surfaces to show up. After review and CI go green, governance also runs
one final polish/simplification builder pass before merge.

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

Preview the next routed issue and profile:

```bash
python3 scripts/conductor.py route-issue \
  --repo misty-step/bitterblossom \
  --label autopilot \
  --limit 25
```

`route-issue` emits JSON with the selected issue, chosen profile, semantic rationale, and any skipped issue numbers keyed to explicit readiness failures. Auto-pick in `run-once`/`loop` uses the same readiness + routing path.

Inspect runs:

```bash
python3 scripts/conductor.py show-runs --limit 20
python3 scripts/conductor.py show-run --run-id run-450-1772813415
python3 scripts/conductor.py show-events --run-id run-450-1772813415
python3 scripts/conductor.py show-workers \
  --repo misty-step/bitterblossom \
  --worker noble-blue-serpent:2 \
  --worker moss \
  --desired-concurrency 2
python3 scripts/conductor.py reset-worker-slots \
  --repo misty-step/bitterblossom \
  --worker noble-blue-serpent \
  --worker moss
```

`show-runs` emits one JSON object per run. The operator contract is that each row includes the current `phase` and `status`, the raw `heartbeat_at` timestamp, a computed `heartbeat_age_seconds`, and when applicable a `blocking_reason` plus the source `blocking_event_type`.

`show-events` emits one JSON object for the requested run with a `run` metadata envelope, `latest_event_type`, `latest_event_at`, and an `events` array. Use it when you need recent event context without joining SQLite tables by hand.

`show-run` is the narrower single-run inspection surface: it returns the same run metadata together with a `recent_events` array keyed by `run_id`.

`show-workers` is the worker-pool admin surface. It returns slot-level health,
current assignments, computed backfill demand against `--desired-concurrency`,
and recent slot-drain / selection events so operators can see which capacity is
healthy before touching sprites manually. On a fresh database it still reports
the configured slots from `--worker ...` even before any run has materialized
those slot rows in SQLite, so the inspection surface stays truthful without
seeding state as a side effect.

`reset-worker-slots` is the recovery surface for drained capacity. It resets the
matching workers back to `active`, clears probe failures, and removes stale
slot assignment state so transient probe failures do not strand worker capacity
forever.

## Acceptance Proof

Issue [#102](https://github.com/misty-step/bitterblossom/issues/102) is the bounded-governance acceptance path for the current conductor architecture.

Run the acceptance-focused regression slice first:

```bash
python3 -m pytest -q scripts/test_conductor.py -k 'acceptance_trace_bullet_run or duplicate_trusted_findings or low_severity_nit or novel_high_severity'
```

Expected:

- the trace bullet path reaches `merged`
- duplicate findings across review surfaces are recorded without reopening the loop
- late low-severity nits are recorded without reopening the loop
- late novel high-severity findings still reopen the loop

Then run the full conductor test file:

```bash
python3 -m pytest -q scripts/test_conductor.py
```

For an operator-visible proof on a prepared environment, execute one run and inspect the run store:

```bash
python3 scripts/conductor.py run-once \
  --repo misty-step/bitterblossom \
  --issue 102 \
  --worker noble-blue-serpent \
  --trusted-external-surface CodeRabbit \
  --reviewer council-fern-20260306 \
  --reviewer council-sage-20260306 \
  --reviewer council-thorn-20260306

python3 scripts/conductor.py show-runs --limit 5
python3 scripts/conductor.py show-run --run-id <run-id>
python3 scripts/conductor.py show-events --run-id <run-id>
```

The acceptance run is only valid if the operator surfaces expose the full path:

- lease acquired
- builder handoff (`phase=awaiting_governance`)
- governance freshness wait / adoption
- review evidence
- CI wait completion
- external review settle or block evidence
- final merge or blocked state

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

Long waits are heartbeat-backed. During governance freshness waits, review dispatch, PR-check polling, and trusted external review polling, the conductor refreshes both the run heartbeat and the lease expiry so a healthy run does not look stale just because GitHub or reviewers are slow.

Builder runs now execute in a run-scoped Git worktree under the warm mirror:

- mirror: `/home/sprite/workspace/<repo>`
- builder worktree: `/home/sprite/workspace/<repo>/.bb/conductor/<run-id>/builder-worktree`
- reviewer worktrees: `/home/sprite/workspace/<repo>/.bb/conductor/<run-id>/review-<reviewer>-worktree`

The shared checkout stays warm for fetches and object reuse, but the conductor no longer reuses it as the execution surface for builder or reviewer runs.

### Worktree Lifecycle and Serialization

Mirror mutation — `git fetch`, `git worktree add/remove`, `git worktree prune` — is serialized at two levels:

1. **Python-level lock**: a `threading.Lock` per `(sprite, mirror)` pair serializes calls within a single conductor process. This protects reviewer and builder lanes that happen to use the same sprite concurrently without stalling independent sprites that have their own mirrors.
2. **Filesystem flock**: the bash script wraps all git mirror operations in `flock --exclusive` on a `.conductor_lock` file inside the mirror directory. This serializes concurrent *processes* (e.g., two supervisors running against the same sprite).

**Prepare retries**: `prepare_run_workspace` retries up to `WORKSPACE_PREP_RETRIES` times (default 2) on transient `CmdError`. A run that cannot prepare its workspace after all retries fails with an explicit `workspace preparation failed after N attempts: <root cause>` message rather than leaving ambiguous state.

**Cleanup failure visibility**: if workspace cleanup fails after a run ends, the conductor records a `workspace_cleanup_failed` event with:
- `error`: the failure message
- `surviving_path`: the worktree path that was not removed

The `worktree_path` column in the run store is intentionally **not cleared** when cleanup fails. An operator can see the surviving path without touching the sprite filesystem.

To inspect worktree state for any run:

```bash
python3 scripts/conductor.py show-runs --limit 5
python3 scripts/conductor.py show-run --run-id <run-id>
```

Both commands include `worktree_path` in the JSON output. A non-null `worktree_path` on a terminal run (merged, failed) usually indicates cleanup did not complete — use `show-events` to see the `workspace_cleanup_failed` event. If the physical cleanup succeeded but a later run-state write failed, the run raises and the last persisted `worktree_path` can be stale until an operator reconciles it.

Manual cleanup on the sprite:

```bash
sprite exec <sprite> -- bash -lc 'git -C /home/sprite/workspace/<repo-name> worktree remove --force <worktree_path>; git -C /home/sprite/workspace/<repo-name> worktree prune'
```

### Builder handoff boundary

Once a builder writes its artifact and the referenced PR is verified, the conductor persists `phase=awaiting_governance` and `pr_number` immediately. That write is the durable boundary between builder work and control-plane cleanup.

Post-artifact sprite cleanup is best-effort. Transport failures during cleanup (e.g., `use of closed network connection`) are recorded as `cleanup_warning` events and do **not** overwrite the run to `phase=failed` or clear `pr_number`.

If a run shows `phase=awaiting_governance` with a valid `pr_number`, the builder delivered its handoff correctly. The operator can run `govern-pr`, reconcile the run, or let a later conductor invocation adopt the PR.

Review state is now split deliberately:

- `reviews` keeps the latest per-reviewer council snapshot for compatibility with existing run logic.
- `review_waves` is append-only wave history for council rounds and PR-thread scans.
- `review_wave_reviews` stores per-wave reviewer verdicts and raw payloads.
- `review_findings` stores normalized findings with reviewer, wave, source id, fingerprint, classification, severity, decision, and status.

Council artifact writes are atomic at the storage boundary: the compatibility snapshot, per-wave reviewer payload, and normalized findings land together for each artifact, and PR-thread scans only finalize their wave after the finding write succeeds.

That split keeps merge policy and GitHub thread mechanics out of the storage contract. Future governance changes can reason over the ledger without losing prior review history.

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
The same `show-runs` row also includes `blocking_reason` so operators can see
the immediate cause without digging through raw events first.

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
python3 scripts/conductor.py show-run --run-id <run-id>
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

If a run is stuck (sprite unresponsive, lease expired), the conductor reclaims the expired lease when that issue is selected again. The previous run records a `lease_stale_reclaimed` event and the replacement run records `lease_reclaimed`, so `show-events` tells you exactly why the issue restarted. To force-release a lease manually:

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

The governor lane does not merge on the first green snapshot. It waits for the configured minimum PR age, ensures required checks are present, queries unresolved review threads before and after trusted external review settlement, routes trusted feedback back to the builder on the existing PR, and only proceeds once the thread gate is clear.

After the PR is green and thread-clear, the governor runs one final polish/simplification pass on the existing PR and re-verifies the review + CI path before squash merge. If the same threads still block after a revision pass, the conductor stops with `pr_feedback_blocked` and escalates to a human for confirmation.

## Review Council

Reviewer dispatches now run in parallel, not serially. Each reviewer writes its own artifact, the conductor persists that result immediately, and `review_complete` events land as each artifact arrives. One slow reviewer no longer hides the rest of the council's progress.

## Prompt Input Trust Model

The conductor constructs prompts from several sources. Not all of them are trusted.

| Input | Source | Trusted? | Handling |
|---|---|---|---|
| `issue.title`, `issue.body` | GitHub Issues (public, user-controlled) | **No** | JSON-fenced + untrusted-data header via `wrap_untrusted_issue_content` |
| `issue.url` | GitHub Issues (system-generated from repo + issue number) | Yes | Embedded plain-text |
| PR review thread comments (`source=pr_review_threads`) | External GitHub reviewers / bots | **No** | JSON-fenced + untrusted-data header via `format_builder_feedback` |
| Internal sprite review summary (`source=review`) | Trusted conductor-owned sprites | Yes | Embedded plain-text |
| Run metadata (run ID, branch, artifact path) | Conductor internals | Yes | Embedded plain-text |

**Rule:** any user-authored string that originates outside the conductor (GitHub issue text, external review bot feedback) is wrapped in a JSON code-fence before being placed in a prompt. The wrapper includes an explicit instruction telling the agent to treat the block as data, not as executable guidance.

The `wrap_untrusted_issue_content` helper ([`scripts/conductor.py`](../scripts/conductor.py)) implements this for issue content. `format_builder_feedback` implements the same pattern for PR review thread feedback.

## MVP Limits

- one builder per issue
- one review council round loop
- one PR-feedback revision loop
- SQLite only
- single-tenant worker assumption
- issue readiness requires `## Product Spec` and `### Intent Contract`
- deterministic lease safety plus Claude-backed semantic ranking

The accepted next cuts are in [ADR-003](./adr/003-conductor-control-plane.md): stale-lease reclaim, resume-first reconciliation, and parallel variants.
