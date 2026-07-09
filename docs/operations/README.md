# Bitterblossom Operations

These runbooks are the operator contract for the Fly-hosted plane. They are
safe to paste into a terminal after replacing the environment variables; do not
put bearer tokens in URLs or command arguments that will be logged.

Canonical app and state:

```sh
export BB_FLY_APP=bitterblossom-plane
export BB_URL=https://bitterblossom-plane-9xpa5.ondigitalocean.app
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
./target/debug/bb --config "$BB_RUNTIME_PLANE" doctor --expect-serve --json
```

`bb doctor --expect-serve` folds the config/db/secrets/binaries checks above
plus a live probe of the running plane's unauthenticated `/health` and `/`
routes into one pass/fail verdict — the fast "is this runtime plane actually
healthy end to end" gate before the itemized checks, not a replacement for
reading them.

Before running GitHub-backed BB tasks, make `GH_TOKEN` explicit for the command
that fans out the work:

```sh
GH_TOKEN=$(gh auth token) ./target/debug/bb --config "$BB_RUNTIME_PLANE" run verify \
  --payload '{"submission":"<submission>"}' \
  --json
```

If a task needs `OPENROUTER_API_KEY`, `GH_TOKEN`, `CERBERUS_REVIEW_GH_TOKEN`,
`BB_API_TOKEN`, or `SPRITE_TOKEN`, missing env should be treated as an operator
preflight failure, not as a useful agent run.

### Cerberus Review Identity And Keys

The `review` task posts through Cerberus and must not use the operator's
personal `GH_TOKEN`. Configure a GitHub App installation token or
least-privilege machine-user token as `CERBERUS_REVIEW_GH_TOKEN` on the runtime
plane. Recommended repository permissions:

- Metadata: read
- Contents: read
- Pull requests: read/write
- Commit statuses: read/write for `CERBERUS_SUMMARY_TARGET=status`
- Checks: read/write only if the runtime task switches to
  `CERBERUS_SUMMARY_TARGET=check-run`

For GitHub App rotation, mint a fresh installation token outside the agent run,
import it by name, restart/recover the plane, then verify the next review
comment identity:

```sh
printf 'CERBERUS_REVIEW_GH_TOKEN=%s\n' "$CERBERUS_REVIEW_GH_TOKEN" \
  | flyctl secrets import --app "$BB_FLY_APP"
flyctl ssh console --app "$BB_FLY_APP" --command '/bin/sh -lc "BB_PLANE_DIR=${BB_PLANE_DIR:-/app/plane} bb recover --json"'
gh api repos/<owner>/<repo>/issues/<pr>/comments --jq '.[].user.login'
```

For a machine-user fallback, use a fine-grained PAT restricted to the reviewed
repositories and the same permissions. The fallback is acceptable only while
the distinct bot identity is visible in the `gh api ... user.login` readback.

The review model key path is also name-only, but it needs **two distinct**
OpenRouter credentials, not one -- see bitterblossom-942 below for why. The
runtime `agents/cerberus-reviewer.toml` should declare:

```toml
secrets = [
  "CERBERUS_REVIEW_GH_TOKEN",
  "OPENROUTER_API_KEY",
  "CERBERUS_OPENROUTER_PROVISIONING_KEY_ENV",
  "CERBERUS_REVIEW_OPENROUTER_PROVISIONING_KEY",
]
```

Optionally, BB's own per-workload capped-key minting can also govern the plain
`OPENROUTER_API_KEY` chat-completion path via:

```toml
[policy]
provider_key_name = "cerberus-reviewer"
provider_spend_cap_usd = 1.25
trigger_bindings = ["manual", "webhook"]
side_effect_policy = "kill"
```

Before enabling review dispatch after a fresh plane setup or key rotation:

```sh
OPENROUTER_MANAGEMENT_KEY="$OPENROUTER_MANAGEMENT_KEY" \
  ./target/debug/bb --config "$BB_RUNTIME_PLANE" keys mint cerberus-reviewer --json
./target/debug/bb --config "$BB_RUNTIME_PLANE" keys sync cerberus-reviewer --check --json
CERBERUS_REVIEW_GH_TOKEN="$CERBERUS_REVIEW_GH_TOKEN" \
  ./target/debug/bb --config "$BB_RUNTIME_PLANE" preflight review --json
```

**bitterblossom-942 (2026-07-09):** the design previously documented here
conflated two different OpenRouter key types and 401'd every review run past
the GH-auth check. OpenRouter's `/keys` management API (list/create/delete
scoped keys) requires a genuine **Provisioning/Management API key** -- the
same kind as `OPENROUTER_MANAGEMENT_KEY`, mintable only from the OpenRouter
dashboard, and unable to make inference calls itself. A regular, capped
inference key -- which is all `OPENROUTER_API_KEY` ever is, whether it is the
plane's shared key or a `bb keys mint`-minted per-workload key -- **cannot**
call `/keys`; OpenRouter returns 401. Cerberus's `--openrouter-scoped-key`
flow needs the former to mint the latter (a per-review, capped, host-side-only
key it then shadows into `OPENROUTER_API_KEY` for the sandboxed substrate), so
it must be fed a real provisioning key through a name distinct from
`OPENROUTER_API_KEY`:

- `CERBERUS_OPENROUTER_PROVISIONING_KEY_ENV` -- a plain (non-secret) value
  naming the env var Cerberus should read as its provisioning-key source;
  live value: `CERBERUS_REVIEW_OPENROUTER_PROVISIONING_KEY`. The wrapper
  (`scripts/cerberus-review-wrapper.sh`) forwards this name straight to
  Cerberus's `--openrouter-provisioning-key-env` flag; no wrapper code
  change was needed to fix this.
- `CERBERUS_REVIEW_OPENROUTER_PROVISIONING_KEY` -- the actual Provisioning
  key value. Currently the same value as `OPENROUTER_MANAGEMENT_KEY`,
  declared under its own name for auditability and so a future dedicated
  dashboard-minted key can be swapped in without touching code.

`OPENROUTER_MANAGEMENT_KEY` itself remains `bb keys`-provisioning-only and is
not declared on the review task.

## Live Plane Config On DO App Platform (Ephemeral Disk)

DO App Platform gives `bitterblossom-plane` no persistent volume, so
`plane.toml`/`agents/`/`tasks/` cannot live baked into the image or on a
mounted disk the way they did on the Fly volume above. Instead,
`scripts/bb-litestream-entrypoint.sh` fetches and unpacks a tarball from
`BB_PLANE_CONFIG_URL` (a presigned Spaces link, set as an app-spec secret)
into `$BB_PLANE_DIR` on every fresh container boot, only when `plane.toml` is
absent. The tarball itself is the actual source of truth for what workload
config is live — this repo's local `plane/` directory (gitignored) is a
separate dev/test plane, not a mirror of it.

Current location: `s3://misty-step-litestream/bb-plane/plane-config-live.tgz`
(DO Spaces, `nyc3`; `DO_SPACES_KEY`/`DO_SPACES_SECRET` sourced from
`~/.secrets`). To change a live agent or task config:

```sh
AWS_ACCESS_KEY_ID="$DO_SPACES_KEY" AWS_SECRET_ACCESS_KEY="$DO_SPACES_SECRET" \
  aws --endpoint-url https://nyc3.digitaloceanspaces.com s3 cp \
  s3://misty-step-litestream/bb-plane/plane-config-live.tgz plane-config-live.tgz
mkdir extracted && tar xzf plane-config-live.tgz -C extracted
# edit extracted/agents/*.toml or extracted/tasks/*/task.toml
cd extracted
COPYFILE_DISABLE=1 tar --no-xattrs --no-acls --no-mac-metadata \
  -czf ../plane-config-live-new.tgz agents tasks plane.toml
cd ..
AWS_ACCESS_KEY_ID="$DO_SPACES_KEY" AWS_SECRET_ACCESS_KEY="$DO_SPACES_SECRET" \
  aws --endpoint-url https://nyc3.digitaloceanspaces.com s3 cp \
  plane-config-live-new.tgz s3://misty-step-litestream/bb-plane/plane-config-live.tgz
```

**macOS tar gotcha:** building the replacement tarball with macOS's default
`tar`/`bsdtar` silently embeds AppleDouble metadata as PAX extended headers
and, more dangerously, as sidecar entries (`._foo.toml` alongside every real
`foo.toml`). GNU tar on the container only warns
(`Ignoring unknown extended header keyword
'LIBARCHIVE.xattr.com.apple.provenance'`) and extracts both — but a sidecar
still matches `*.toml`, and `bb`'s config loader crashes trying to read its
binary AppleDouble content as UTF-8 TOML, taking down the whole container at
boot (`DeployContainerExitNonZero`, readiness probe failures). Always build
with `COPYFILE_DISABLE=1 tar --no-xattrs --no-acls --no-mac-metadata` and
verify `tar tzf <bundle> | grep -c '\._'` is `0` before uploading; a Docker
`debian:bookworm-slim` extraction (matching the production base image) is a
cheap way to confirm before pushing to Spaces.

An app-spec-only change (e.g. a new secret) still triggers a full redeploy,
which re-fetches this bundle onto the fresh ephemeral disk — so a bundle fix
and a spec-only fix land together on the next `doctl apps update`. DO
auto-rolls back to the last healthy deployment on `DeployContainerExitNonZero`
with no operator action needed; confirm via
`doctl apps list-deployments <app-id>` and diff `get-deployment ... -o json`
step statuses before assuming a bad deploy is still live.

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
inside the machine. Fly SSH commands that include env assignment or shell
syntax must be wrapped with `/bin/sh -lc`; otherwise Fly tries to execute the
assignment as the binary name.

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

The production container starts through `bb-litestream-entrypoint`. Fly keeps
the instance data volume mounted at `/app/plane` and sets the Litestream runtime
contract by env name only:

```sh
BB_LITESTREAM_REQUIRED=1
BB_LITESTREAM_DB_PATH=/app/plane/.bb/plane.db
BB_LITESTREAM_CONFIG=/tmp/bb-litestream.yml
BB_LITESTREAM_REPLICA_URL_ENV=LITESTREAM_REPLICA_URL
BB_LITESTREAM_HEARTBEAT_PATH=/app/plane/.bb/backup-last-success
BB_LITESTREAM_STARTUP_TIMEOUT_SECONDS=60
```

Set the actual replica URL as a Fly secret, never in git:

```sh
printf 'LITESTREAM_REPLICA_URL=%s\n' "$LITESTREAM_REPLICA_URL" \
  | flyctl secrets import --app "$BB_FLY_APP"
```

On startup, the entrypoint writes a temporary Litestream config containing
`${LITESTREAM_REPLICA_URL}`, starts `litestream replicate -config`, waits for an
initial `litestream sync -wait`, and writes the heartbeat only after sync
confirms a durable remote write. If `BB_LITESTREAM_REQUIRED=1` and the secret is
missing, the initial sync does not complete, or Litestream exits while
`bb serve` is still running, the container exits instead of accepting
unprotected work.

## Rollback

Every `bb` binary stamps the SQLite ledger with `PRAGMA user_version` and
refuses to open a ledger whose `ledger.schema_version` is newer than the binary
supports. That is the old-binary/new-schema rollback contract: additive schema
changes are rollback-safe only while the older binary supports the same ledger
version; otherwise the safe moves are roll forward or restore a compatible
backup. Do not edit `PRAGMA user_version` to force an old binary to write into a
newer ledger.

Before rolling back, capture the current app and ledger version:

```sh
flyctl releases --app "$BB_FLY_APP"
{
  printf '%s\n' 'fail'
  printf '%s\n' 'silent'
  printf '%s\n' 'show-error'
  printf 'url = "%s/api/status"\n' "$BB_URL"
  printf 'header = "Authorization: Bearer %s"\n' "$BB_API_TOKEN"
} | curl --config -
```

If `ledger.schema_version` is newer than the rollback target supports, restore
a backup from before that migration or roll forward to a binary that supports
the new schema. If the schema is compatible, pick the previous known-good
release, rollback, recover inherited runs, and run the smoke:

```sh
flyctl releases --app "$BB_FLY_APP"
flyctl releases rollback --app "$BB_FLY_APP" --yes
flyctl ssh console --app "$BB_FLY_APP" --command '/bin/sh -lc "BB_PLANE_DIR=${BB_PLANE_DIR:-/app/plane} bb recover --json"'
BB_API_TOKEN="$BB_API_TOKEN" ./scripts/production-ops-drill.sh --remote
```

If rollback does not restore `/health` and bearer read APIs, stop dispatching
new work and inspect machine logs:

```sh
flyctl logs --app "$BB_FLY_APP"
flyctl status --app "$BB_FLY_APP"
```

If logs contain `newer than this bb binary supports`, the rollback binary is
correctly refusing a newer ledger. Roll forward or restore a compatible backup;
do not force the schema version downward.

## Restart Recovery

After a restart or deploy, classify inherited running rows:

```sh
flyctl ssh console --app "$BB_FLY_APP" --command '/bin/sh -lc "BB_PLANE_DIR=${BB_PLANE_DIR:-/app/plane} bb recover --json"'
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

Production replication is Litestream-backed. To prove the current replica before
an incident, run a dry-run restore inside the machine and read the backup health
surface:

```sh
flyctl ssh console --app "$BB_FLY_APP" --command '
  litestream restore -config /tmp/bb-litestream.yml -dry-run -json /app/plane/.bb/plane.db
  BB_PLANE_DIR=/app/plane bb status --json
'
```

For an operator-held backup copy, copy the volume-backed SQLite database from
inside the Fly machine, then verify the copy locally before it is trusted:

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
backup or restore from the Litestream replica to the mounted path, restart, then
run the smoke:

```sh
flyctl machines stop --app "$BB_FLY_APP" <machine-id>
flyctl ssh console --app "$BB_FLY_APP" --command '
  rm -f /app/plane/.bb/plane.db /app/plane/.bb/plane.db-wal /app/plane/.bb/plane.db-shm
  litestream restore -config /tmp/bb-litestream.yml -json /app/plane/.bb/plane.db
'
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
  printf 'CERBERUS_REVIEW_GH_TOKEN=%s\n' "$CERBERUS_REVIEW_GH_TOKEN"
  printf 'SPRITE_TOKEN=%s\n' "$SPRITE_TOKEN"
  printf 'LITESTREAM_REPLICA_URL=%s\n' "$LITESTREAM_REPLICA_URL"
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

Before unparking a task that has been parked for a while, inspect the held
backlog:

```bash
bb runs list --task <task> --state blocked_budget --json
```

Retire runs targeting closed, merged, superseded, or otherwise stale externals
with `bb runs retire <id> --reason "<why>"`. Prefer `bb runs release <id>` for
one live run or `bb task unpark <task> --since <timestamp> --yes` for a bounded
recent slice. A blind multi-run `bb task unpark <task>` requires `--yes` after
printing the blocked count and age range; treat that preview as the final
operator check before re-queueing historical work.
