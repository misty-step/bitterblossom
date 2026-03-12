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

`show-events` emits one JSON object for the requested run with a `run` metadata envelope, `latest_event_type`, `latest_event_at`, and an `events` array. Review convergence is now explicit in that stream: `review_wave_started`, `review_wave_completed`, and `external_review_wait_complete` events let operators inspect when a council round began, when a PR-thread scan or external-review wait settled, and why governance advanced or stopped.

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
python3 -m pytest -q scripts/test_conductor.py -k 'acceptance_trace_bullet_run or duplicate_fingerprint or low_severity_nit or novel_high_severity or trusted_thread'
```

Expected:

- the trace bullet path reaches `merged`
- duplicate findings across reviewers, review waves, and trusted PR-thread surfaces are recorded without reopening the loop
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
- explicit review-wave start/finish events for council rounds, PR-thread scans, and trusted external-review settlement
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
  mkdir -p ~/.bb
  cat > ~/.bb/conductor-supervisor.env <<EOF
export GITHUB_TOKEN=...
export SPRITE_TOKEN=...
EOF
  chmod 600 ~/.bb/conductor-supervisor.env
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

### Supported Supervisor Contract

`nohup python3 scripts/conductor.py loop ...` is **not** the supported always-on contract. It survives a disconnected shell, but it does not restart after a crash and it does not come back after a host reboot.

The supported coordinator contract is:

- `scripts/conductor-supervise.sh run ...` owns the long-lived process and restarts `python3 scripts/conductor.py loop ...` after both clean exits and crashes.
- `scripts/conductor-supervise.sh install-cron ...` installs a user `@reboot` entry that relaunches the supervisor after coordinator reboot.
- The reboot launcher sources `~/.bb/conductor-supervisor.env` before starting the supervisor, so tokens are available to cron's non-interactive shell.
- Supervisor state lives under `~/.bb/conductor-supervisor/` with a stable `current.log`, `supervisor.pid`, `child.pid`, and `launch.sh`.
- Logs are bounded locally: when `current.log` reaches `10 MiB` (override with `BB_CONDUCTOR_LOG_MAX_BYTES`), the supervisor rotates it to `conductor-YYYYmmdd-HHMMSS.log` and keeps the newest `10` archived files (override with `BB_CONDUCTOR_LOG_KEEP_FILES`).

This keeps the deployment lightweight: one shell supervisor plus cron, no Kubernetes, no separate daemon framework.

### Starting the Loop

Start the supported supervisor on the coordinator:

```bash
sprite exec coordinator -- bash -lc '
  cd /home/sprite/workspace/bitterblossom
  ./scripts/conductor-supervise.sh start \
    --repo misty-step/bitterblossom \
    --label autopilot \
    --worker noble-blue-serpent \
    --reviewer council-fern-20260306 \
    --reviewer council-sage-20260306 \
    --reviewer council-thorn-20260306
'
```

The supervisor keeps the loop alive across crashes. The conductor still polls for eligible issues every 60 seconds (configurable with `--poll-seconds`). Transient failures log and continue; blocked runs (requiring human review) are noted in the issue and the loop moves on.

### Reboot Bootstrap

Install the reboot hook once on the coordinator:

```bash
sprite exec coordinator -- bash -lc '
  cd /home/sprite/workspace/bitterblossom
  ./scripts/conductor-supervise.sh install-cron \
    --repo-root /home/sprite/workspace/bitterblossom \
    --repo misty-step/bitterblossom \
    --label autopilot \
    --worker noble-blue-serpent \
    --reviewer council-fern-20260306 \
    --reviewer council-sage-20260306 \
    --reviewer council-thorn-20260306
'
```

Equivalent local entrypoints exist through `make conductor-start`, `make conductor-install-cron`, `make conductor-status`, and `make conductor-stop` with `CONDUCTOR_SUPERVISOR_ARGS='...'`.

### Sleep and Lifecycle Assumptions

- The coordinator should run on a dedicated remote sprite, not on a laptop shell. Laptop sleep is out of path once the remote supervisor is started.
- Worker sprites may sleep when idle. The conductor coordinator should not depend on an attached interactive session.
- If the coordinator host reboots, cron relaunches `launch.sh`, which restarts the supervisor, which restarts the conductor loop.

### Verifying the Loop

Check the supervisor and reboot hook:

```bash
sprite exec coordinator -- bash -lc '
  cd /home/sprite/workspace/bitterblossom
  ./scripts/conductor-supervise.sh status
  crontab -l | grep conductor-supervisor/launch.sh
'
```

Check run state:

```bash
sprite exec coordinator -- bash -lc '
  cd /home/sprite/workspace/bitterblossom
  python3 scripts/conductor.py show-runs --limit 5
'
```

Tail the bounded supervisor log:

```bash
sprite exec coordinator -- bash -lc 'tail -f ~/.bb/conductor-supervisor/current.log'
```

### Durable Run State

Every run writes immediately to `.bb/conductor.db` and `.bb/events.jsonl` on the coordinator. State survives supervisor restarts and coordinator reboots. Already-completed runs will not be re-processed just because the loop was restarted because their leases have been released.

Long waits are heartbeat-backed. During governance freshness waits, review dispatch, PR-check polling, and trusted external review polling, the conductor refreshes both the run heartbeat and the lease expiry so a healthy run does not look stale just because GitHub or reviewers are slow.

Builder runs now execute in a run-scoped Git worktree under the warm mirror:

- mirror: `/home/sprite/workspace/<repo>`
- builder worktree: `/home/sprite/workspace/<repo>/.bb/conductor/<run-id>/builder-worktree`
- reviewer worktrees: `/home/sprite/workspace/<repo>/.bb/conductor/<run-id>/review-<reviewer>-worktree`

The shared checkout stays warm for fetches and object reuse, but the conductor no longer reuses it as the execution surface for builder or reviewer runs.

### Worktree Lifecycle and Serialization

Mirror mutation is conductor-owned and serialized on-sprite with a per-repo lock. `fetch --all --prune`,
`worktree add`, `worktree remove`, and `worktree prune` no longer race each other across overlapping runs on one sprite.

Workspace preparation now retries transient sprite/git failures up to three attempts before the run fails with an explicit
`workspace_preparation_failed` event. Operators should treat that event as the authoritative failure reason for the
specific workspace/lane that exhausted retries, including reviewer and governance preparation paths, not as a generic
command failure.

To inspect which builder worktree a completed run used on the worker:

```bash
python3 scripts/conductor.py show-runs --limit 5
python3 scripts/conductor.py show-run --run-id <run-id>
```

The JSON row now includes the persisted builder `worktree_path` plus explicit recovery fields:

- `worktree_recovery_status` — `cleaned`, `cleanup_failed`, or `prepare_failed`
- `worktree_recovery_error` — the last cleanup/preparation error when recovery degraded
- `worktree_recovery_event_type` / `worktree_recovery_event_at` — the event that established that recovery state

If builder cleanup fails, `worktree_path` remains populated so the surviving builder worktree can be inspected and recovered
without reading the sprite filesystem first. Reviewer cleanup and reviewer workspace-preparation failures stay in the event
ledger rather than the top-level run row.

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
- `review_waves` is append-only wave history for council rounds, PR-thread scans, and trusted external-review settlement waits.
- `review_wave_reviews` stores per-wave reviewer verdicts and raw payloads.
- `review_findings` stores normalized findings with reviewer, wave, source id, fingerprint, classification, severity, decision, and status. Duplicate fingerprints now collapse across review surfaces so a repeated blocker is recorded once semantically instead of reopening the run on every restatement.

Council artifact writes are atomic at the storage boundary: the compatibility snapshot, per-wave reviewer payload, and normalized findings land together for each artifact, and PR-thread scans only finalize their wave after the finding write succeeds.

That split keeps merge policy and GitHub thread mechanics out of the storage contract. Future governance changes can reason over the ledger without losing prior review history.

### Migration Note: Trusted Duplicate Threads

Issue [#500](https://github.com/misty-step/bitterblossom/issues/500) changes one trusted-review behavior deliberately: if a trusted PR thread restates a finding that is already active in the review ledger, the new thread finding is recorded as `duplicate` instead of reopening the full governance loop by itself.

Operator verification:

- inspect `show-events` for the matching `review_wave_completed` PR-thread scan event
- inspect `review_findings` or run acceptance-focused tests to confirm the repeated thread is stored as `duplicate`
- monitor runs that used to reopen on trusted restatements and confirm they now reopen only for genuinely novel or still-unresolved threads

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
sprite exec coordinator -- bash -lc 'tail -50 ~/.bb/conductor-supervisor/current.log'
```

Fix the root cause, then restart the supervisor:

```bash
sprite exec coordinator -- bash -lc '
  cd /home/sprite/workspace/bitterblossom
  ./scripts/conductor-supervise.sh stop
  ./scripts/conductor-supervise.sh start \
    --repo misty-step/bitterblossom \
    --label autopilot \
    --worker noble-blue-serpent \
    --reviewer council-fern-20260306 \
    --reviewer council-sage-20260306 \
    --reviewer council-thorn-20260306
'
```

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
