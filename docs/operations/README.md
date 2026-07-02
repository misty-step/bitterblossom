# Bitterblossom Operations

These runbooks are the operator contract for the Fly-hosted plane. They are
safe to paste into a terminal after replacing the environment variables; do not
put bearer tokens in URLs or command arguments that will be logged.

Canonical app and state:

```sh
export BB_FLY_APP=bitterblossom-plane
export BB_URL=https://bitterblossom-plane.fly.dev
export BB_RUNTIME_PLANE=/path/to/private/plane
```

The production instance plane lives on the Fly volume mounted at `/app/plane`.
That volume contains runtime config (`plane.toml`, `agents/`, `tasks/`) plus
state (`.bb/plane.db`, child-key metadata, run artifacts). The Docker image must
not contain the production `plane/` directory.

For the local reviewer dashboard served on Serenity over Tailscale, see
[`bb-dashboard.md`](bb-dashboard.md). That service intentionally runs a local
dev/demo plane rather than the production plane.

## Preflight

Before dispatching a storm or touching production, check the operator surface:

```sh
git status --short --branch --untracked-files=all
flyctl orgs list
sprite org list
sprite use -o misty-step lane-1
sprite org list
sprite exec -- whoami
./target/debug/bb --config "$BB_RUNTIME_PLANE" check
./target/debug/bb --config "$BB_RUNTIME_PLANE" status --json
./target/debug/bb --config "$BB_RUNTIME_PLANE" dlq list --json
./target/debug/bb --config "$BB_RUNTIME_PLANE" notify list --json
```

Before running GitHub-backed BB tasks, make `GH_TOKEN` explicit for the command
that fans out the work:

```sh
GH_TOKEN=$(gh auth token) ./target/debug/bb --config "$BB_RUNTIME_PLANE" run verify \
  --payload '{"submission":"<submission>"}' \
  --json
```

If a task needs `OPENROUTER_API_KEY`, `GH_TOKEN`, `BB_API_TOKEN`, or
`SPRITE_TOKEN`, missing env should be treated as an operator preflight failure,
not as a useful agent run.

## Deploy

Local gate first:

```sh
./scripts/verify.sh
```

Deploy only from a clean, pushed `master`:

```sh
git status --short --branch --untracked-files=all
git rev-list --left-right --count master...origin/master
flyctl deploy --app "$BB_FLY_APP" --remote-only
```

For a first deploy or a migration from the old image-baked plane, stage the
instance plane on the volume before deploying an image that expects
`BB_PLANE_DIR=/app/plane`:

```sh
# Run against the old deployment where /app/plane is image-backed and
# /app/plane/.bb is the mounted volume root.
flyctl ssh console --app "$BB_FLY_APP" --command '
  set -eu
  cd /app/plane/.bb
  mkdir -p .bb
  find . -mindepth 1 -maxdepth 1 ! -name .bb -exec mv {} .bb/ \;
  cp -a /app/plane/plane.toml /app/plane/agents /app/plane/tasks .
  test -f plane.toml
  test -d agents
  test -d tasks
  test -f .bb/plane.db
'
```

After that migration, the Fly volume can be mounted at `/app/plane`; budget,
task-card, and allowlist changes are applied by updating runtime config on the
volume and restarting/recovering the plane, not by rebuilding the product image.

Run the production smoke after deploy:

```sh
BB_API_TOKEN="$BB_API_TOKEN" \
  ./scripts/production-ops-drill.sh --remote --url "$BB_URL" --fly-app "$BB_FLY_APP"
```

The smoke checks unauthenticated `/health`, bearer-only read APIs, Fly app and
volume visibility, and `BB_PLANE_DIR=${BB_PLANE_DIR:-/app/plane} bb recover --json`
inside the machine.

`bb status --json` also reports backup readiness from the runtime plane's
`[backup]` stanza. The plane should declare the backup provider, RPO/RTO, the
replica secret env name, and a heartbeat file written by the replication
process:

```toml
[backup]
enabled = true
provider = "litestream"
replica_env = "LITESTREAM_REPLICA_URL"
last_success_path = ".bb/backup-last-success"
rpo_seconds = 300
rto_seconds = 1800
```

Healthy output has `backup.status == "fresh"` and `backup.healthy == true`.
Treat `stale`, `unknown`, or a missing heartbeat as a production-readiness
failure before raising unattended workload volume.

## Rollback

List recent releases, pick the previous known-good version, then rollback:

```sh
flyctl releases --app "$BB_FLY_APP"
flyctl releases rollback --app "$BB_FLY_APP" --yes
BB_API_TOKEN="$BB_API_TOKEN" ./scripts/production-ops-drill.sh --remote
```

If rollback does not restore `/health` and bearer read APIs, stop dispatching
new work and inspect machine logs:

```sh
flyctl logs --app "$BB_FLY_APP"
flyctl status --app "$BB_FLY_APP"
```

## Restart Recovery

After a restart or deploy, classify inherited running rows:

```sh
flyctl ssh console --app "$BB_FLY_APP" --command 'BB_PLANE_DIR=${BB_PLANE_DIR:-/app/plane} bb recover --json'
{
  printf '%s\n' 'fail'
  printf '%s\n' 'silent'
  printf '%s\n' 'show-error'
  printf 'url = "%s/api/status"\n' "$BB_URL"
  printf 'header = "Authorization: Bearer %s"\n' "$BB_API_TOKEN"
} | curl --config -
```

Rows in `awaiting_recovery` require side-effect inspection before resolution:

```sh
./target/debug/bb --config "$BB_RUNTIME_PLANE" runs show <run-id> --json
./target/debug/bb --config "$BB_RUNTIME_PLANE" runs resolve <run-id> success --reason "<side effects inspected>"
```

In `recover --json`, treat `probe_state` as the machine-readable recovery
contract. `alive` keeps the run `running` and retains the lease. `dead` moves
to `awaiting_recovery` and releases the lease. `unknown` moves to
`awaiting_recovery` but retains the lease; read `probe_reason` and the
`boot_probe` event before deciding whether the external side effect completed.

Do not replay an at/after-execute run just because it is inconvenient. Only
pre-execute dead letters are mechanically replayable.

## Backup And Restore

Production backup copies the volume-backed SQLite database from inside the Fly
machine, then verifies the copy locally before it is trusted:

```sh
mkdir -p .ops/backups
flyctl ssh sftp shell --app "$BB_FLY_APP"
# Inside sftp:
# get /app/plane/.bb/plane.db .ops/backups/plane-$(date +%Y%m%dT%H%M%SZ).db
python3 - <<'PY'
import sqlite3
db = ".ops/backups/<backup-file>.db"
conn = sqlite3.connect(db)
print(conn.execute("pragma integrity_check").fetchone()[0])
print(conn.execute("select count(*) from runs").fetchone()[0])
PY
```

Restore is a manual incident action. Stop the machine, upload the verified
backup to the mounted path, restart, then run the smoke:

```sh
flyctl machines stop --app "$BB_FLY_APP" <machine-id>
flyctl ssh sftp shell --app "$BB_FLY_APP"
# Inside sftp:
# put .ops/backups/<backup-file>.db /app/plane/.bb/plane.db
flyctl machines start --app "$BB_FLY_APP" <machine-id>
BB_API_TOKEN="$BB_API_TOKEN" ./scripts/production-ops-drill.sh --remote
```

The local restore drill in `./scripts/production-ops-drill.sh --local` proves
the backup mechanism preserves run history without touching production, and it
asserts the backup readiness field through `/api/status`. It also opens a
fixture submission and checks `bb check`, `bb status --json`, `bb runs list
--json`, and `bb gate --change ops-drill --json` against the restored DB copy.

## Secret Rotation

Runtime secrets live in Fly, not git:

```sh
flyctl secrets list --app "$BB_FLY_APP"
{
  printf 'BB_API_TOKEN=%s\n' "$BB_API_TOKEN"
  printf 'OPENROUTER_API_KEY=%s\n' "$OPENROUTER_API_KEY"
  printf 'GH_TOKEN=%s\n' "$GH_TOKEN"
  printf 'SPRITE_TOKEN=%s\n' "$SPRITE_TOKEN"
} | flyctl secrets import --app "$BB_FLY_APP"
```

After rotation, run the smoke. For webhook secrets, also send a signed test
delivery from the upstream provider before declaring the rotation complete.

## Stuck Runs And DLQ Triage

Start with the status surface:

```sh
./target/debug/bb --config "$BB_RUNTIME_PLANE" status --json
./target/debug/bb --config "$BB_RUNTIME_PLANE" dlq list --json
```

Classify each problem:

- **Replayable pre-execute DLQ:** missing secret, failed clone, unavailable
  sprite before the agent executed. Fix the condition, then run
  `bb dlq replay <id> --json`.
- **Superseded pre-execute DLQ:** a replacement submission or run already
  passed. Close it with `bb dlq ack <id> --reason <text>` so it stops
  counting as open operator work; the row is kept with reason and timestamp,
  replay history stays immutable, and an acknowledged DLQ cannot be replayed.
- **At/after-execute uncertainty:** use `bb runs show <run-id> --json`, inspect
  artifacts and external side effects, then resolve only with
  `bb runs resolve`.

Never hide open DLQs in summaries. Acknowledgement is an explicit operator
closure with a recorded reason, not a way to hide failures — if a DLQ is not
known-superseded, replay or resolve the underlying run instead.

## Notification Outbox

Webhook notifications are durable before transport. If `status --json` shows
pending or failed rows under `guards.notify.outbox`, inspect and retry them:

```sh
./target/debug/bb --config "$BB_RUNTIME_PLANE" notify list --json
./target/debug/bb --config "$BB_RUNTIME_PLANE" notify retry --json
```

If the notification has already been handled out of band, close it explicitly:

```sh
./target/debug/bb --config "$BB_RUNTIME_PLANE" notify ack <id> --reason "<handled reason>" --json
```

Acknowledgement keeps the row with reason and timestamp; it is not a delete.

## Attention Debt Brake

`bb status --json` exposes aggregate unattended-loop debt at
`guards.attention_debt`: open DLQs, parked tasks, stale executing runs,
`awaiting_recovery` runs, and pending or failed notification rows. Any nonzero
count makes `blocking=true`.

When the brake is blocking, new reflex admissions are refused before they create
run rows:

- signed webhooks return HTTP `429` with the debt counts;
- serve-mode cron catch-up records an `attention_debt_brake` guard event and
  skips the due fire instead of queueing more work;
- manual `bb run ...` remains available for operator-controlled repair.

Clear the named debt first: replay or acknowledge DLQs, unpark tasks only after
the reason is fixed, inspect/resolve `awaiting_recovery`, run `bb recover --json`
for stale executing work, and retry or acknowledge notification outbox rows.
