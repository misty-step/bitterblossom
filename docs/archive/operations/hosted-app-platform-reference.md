> **Historical reference only — not the current Bitterblossom production contract.**
>
> This document records the retired hosted-plane/App Platform operations path. The shipped service now runs as a local-primary launchd service. Do not use these commands for current operations.

# Bitterblossom Operations

These runbooks are the operator contract for the DigitalOcean App Platform
plane. They are safe to paste into a terminal after replacing the environment
variables; do not put bearer tokens in URLs or command arguments that will be
logged.

Canonical app and state:

```sh
export BB_DO_APP_ID=<operator-local-app-id>
export BB_URL=https://bitterblossom-plane-9xpa5.ondigitalocean.app
export BB_RUNTIME_PLANE=/path/to/private/plane
export BB_DO_APP_SPEC=/path/to/private/bitterblossom-app.yaml
export BB_LITESTREAM_CONFIG_LOCAL=/path/to/private/litestream.yml
```

App Platform's container disk is ephemeral. Runtime config is fetched into
`/app/plane` at boot and ledger state is restored and replicated by Litestream;
the product image must not contain the production `plane/` directory. Do not
describe `/app/plane` as a persistent provider volume.

For the local reviewer dashboard served on Serenity over Tailscale, see
[`bb-dashboard.md`](bb-dashboard.md). That service intentionally runs a local
dev/demo plane rather than the production plane.

## Preflight

Before dispatching a storm or touching production, check the operator surface:

```sh
git status --short --branch --untracked-files=all
doctl apps get "$BB_DO_APP_ID" \
  --format ID,Spec.Name,DefaultIngress,ActiveDeployment.ID,InProgressDeployment.ID
doctl apps list-deployments "$BB_DO_APP_ID" \
  --format ID,Phase,Cause,Created,Updated
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
update the secret in the operator-owned App Platform spec, validate and apply
that complete spec, then verify the next review comment identity. Never put the
token in the command line or a generated patch:

```sh
doctl apps spec validate "$BB_DO_APP_SPEC"
doctl apps update "$BB_DO_APP_ID" --spec "$BB_DO_APP_SPEC" --wait
doctl apps list-deployments "$BB_DO_APP_ID" \
  --format ID,Phase,Cause,Created,Updated
export BB_EXPECTED_DEPLOYMENT_ID=<deployment-id-created-by-this-update>
BB_API_TOKEN="$BB_API_TOKEN" ./scripts/production-ops-drill.sh \
  --remote --url "$BB_URL" --do-app-id "$BB_DO_APP_ID" \
  --expected-deployment-id "$BB_EXPECTED_DEPLOYMENT_ID"
gh api repos/<owner>/<repo>/issues/<pr>/comments --jq '.[].user.login'
```

For a machine-user fallback, use a fine-grained PAT restricted to the reviewed
repositories and the same permissions. The fallback is acceptable only while
the distinct bot identity is visible in the `gh api ... user.login` readback.

The review model key path is also name-only. The runtime
`agents/cerberus-reviewer.toml` should declare
`secrets = ["CERBERUS_REVIEW_GH_TOKEN", "OPENROUTER_API_KEY"]` plus:

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

`OPENROUTER_MANAGEMENT_KEY` is used only by `bb keys` provisioning. It is not a
declared review-run secret. At dispatch, BB injects the stored, capped
per-workload-family key as `OPENROUTER_API_KEY`; the wrapper passes only that
env name to Cerberus `--openrouter-scoped-key`, and Cerberus mints/revokes the
per-review child key inside its isolated `container-opencode` path.

## Live Plane Config On DO App Platform (Ephemeral Disk)

DO App Platform gives `bitterblossom-plane` no persistent volume, so
`plane.toml`/`agents/`/`tasks/` cannot live baked into the image or on a
mounted disk. Instead,
`scripts/bb-litestream-entrypoint.sh` fetches and unpacks a tarball from
`BB_PLANE_CONFIG_URL` (a presigned Spaces link, set as an app-spec secret)
into `$BB_PLANE_DIR` on every fresh container boot, only when `plane.toml` is
absent. The tarball itself is the actual source of truth for what workload
config is live — this repo's local `plane/` directory (gitignored) is a
separate dev/test plane, not a mirror of it.

Current location: `s3://misty-step-litestream/bb-plane/plane-config-live.tgz`
(DO Spaces, `nyc3`; `DO_SPACES_KEY`/`DO_SPACES_SECRET` sourced from
`~/.secrets`). Keep the downloaded original as the known-good rollback bundle
until the replacement deployment and remote smoke are green. To change a live
agent or task config:

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

### Tailnet-only Mint bridge

The DO container reaches the tailnet-only Mint broker through a supervised
userspace Tailscale node, not a public Mint ingress. Set
`BB_MINT_TAILNET_AUTHKEY` to a reusable ephemeral pre-auth key restricted to
the production Bitterblossom tag. `bb-mint-tailnet-entrypoint` writes it to a
mode-0600 temporary file, unsets the environment value, joins the tailnet, and
deletes the file before starting `bb`. The agent process therefore inherits
neither the Tailscale bootstrap key nor a Powder credential.

The root-owned Tailscale socket lives under mode-0700 `/run/bb-mint`; tunnel
processes start with a scrubbed environment. Litestream and `bb` run as the
unprivileged `bb` user with `no-new-privs`, so workloads can reach the
loopback Mint forward but cannot use the Tailscale local API to open arbitrary
tailnet connections. The wrapper does not install an exit node or proxy
unrelated egress.

Before starting `bb`, and every ten seconds afterward, the supervisor performs
a read-only Powder list request through Mint using
`__mint.powder.bitterblossom__`. It exports the same placeholder and Mint proxy
base to `bb`. Loss of tailscaled, the loopback forward, or the end-to-end Mint
Powder capability terminates the container so App Platform restarts the
complete identity boundary.

Before deployment, exercise the real production image boundary (real
`setpriv`, filesystem permissions, `socat`, `curl`, and entrypoints; only the
external Tailscale/Mint network is replaced):

```sh
scripts/mint-tailnet-container-smoke.sh
```

### Tailnet Linejam alert runner

The deterministic `linejam-alert` task runs on its dedicated tailnet host;
generic incident remediation remains on Sprites. The runtime image includes
`openssh-client`. Set `BB_TAILNET_SSH_PRIVATE_KEY` and a pinned
`BB_TAILNET_SSH_KNOWN_HOSTS` value as app secrets. At startup the entrypoint
materializes them as `/root/.ssh/id_ed25519` and `/root/.ssh/known_hosts` with
directory mode `0700` and file mode `0600`, without logging either value. It
does not run `ssh-keyscan`, and it does not touch `/root/.ssh` when both values
are unset.

Canary uses one service-scoped Linejam webhook subscription to the
`linejam-alert` route for `incident.opened`, `incident.updated`, and
`incident.resolved`, with its generated secret stored as
`BB_HOOK_LINEJAM_ALERT`. Do not add a second recovery subscription. The generic
`incident-triage` subscription accepts only opened/updated events for Canary,
Bastion, and Powder.

## Deploy

Local gate first:

```sh
./scripts/verify.sh
```

App Platform follows `misty-step/bitterblossom:master` with deploy-on-push.
Production therefore deploys only from a clean, reviewed, pushed `master`:

```sh
git status --short --branch --untracked-files=all
git rev-list --left-right --count master...origin/master
git push origin master
```

Do not declare success from the push. Read the deployment list and set
`BB_EXPECTED_DEPLOYMENT_ID` to the exact row whose `Cause` names the commit just
pushed. Never substitute whichever deployment happens to remain active: a
failed rollout can leave the previous deployment active. Then exercise the
provider, public, and authenticated read surfaces:

```sh
doctl apps list-deployments "$BB_DO_APP_ID" \
  --format ID,Phase,Cause,Created,Updated
export BB_EXPECTED_DEPLOYMENT_ID=<deployment-id-for-the-pushed-commit>
doctl apps get "$BB_DO_APP_ID" \
  --format ID,Spec.Name,DefaultIngress,ActiveDeployment.ID,InProgressDeployment.ID
doctl apps get-deployment "$BB_DO_APP_ID" "$BB_EXPECTED_DEPLOYMENT_ID" \
  --format ID,Phase,Cause,Created,Updated
```

```sh
BB_API_TOKEN="$BB_API_TOKEN" \
  ./scripts/production-ops-drill.sh --remote --url "$BB_URL" \
  --do-app-id "$BB_DO_APP_ID" \
  --expected-deployment-id "$BB_EXPECTED_DEPLOYMENT_ID"
```

The smoke checks unauthenticated `/health`, bearer-only read APIs, the canonical
App Platform app identity, ingress URL, and the exact expected deployment. It
fails while any deployment is in progress, if the expected deployment is not
the active deployment, or if that deployment is not `ACTIVE`; therefore a
failed rollout that leaves the previous deployment active cannot pass. It does
not open a provider console or mutate runtime state.

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

The production container starts through `bb-litestream-entrypoint`. App
Platform sets the Litestream runtime contract by env name only; every path below
is on ephemeral container storage:

```sh
BB_LITESTREAM_REQUIRED=1
BB_LITESTREAM_DB_PATH=/app/plane/.bb/plane.db
BB_LITESTREAM_CONFIG=/tmp/bb-litestream.yml
BB_LITESTREAM_REPLICA_URL_ENV=LITESTREAM_REPLICA_URL
BB_LITESTREAM_HEARTBEAT_PATH=/app/plane/.bb/backup-last-success
BB_LITESTREAM_STARTUP_TIMEOUT_SECONDS=60
```

Set the actual replica URL as an App Platform secret in the operator-owned app
spec, never in git. Validate the complete spec before applying it:

```sh
doctl apps spec validate "$BB_DO_APP_SPEC"
doctl apps update "$BB_DO_APP_ID" --spec "$BB_DO_APP_SPEC" --wait
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

Before rolling back, capture the current deployment and ledger version:

```sh
doctl apps list-deployments "$BB_DO_APP_ID" \
  --format ID,Phase,Cause,Created,Updated
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
the new schema.

If the failure came from the Spaces-hosted runtime config, upload the downloaded
known-good bundle and force a fresh deployment so the ephemeral container
fetches it before serving traffic:

```sh
AWS_ACCESS_KEY_ID="$DO_SPACES_KEY" AWS_SECRET_ACCESS_KEY="$DO_SPACES_SECRET" \
  aws --endpoint-url https://nyc3.digitaloceanspaces.com s3 cp \
  plane-config-live.tgz \
  s3://misty-step-litestream/bb-plane/plane-config-live.tgz
doctl apps create-deployment "$BB_DO_APP_ID" --force-rebuild --wait
```

If the schema is compatible and the failure came from source, revert the
offending commit on `master`; App Platform deploy-on-push will build a new
deployment from the known-good source state:

```sh
git revert <bad-commit>
git push origin master
doctl apps list-deployments "$BB_DO_APP_ID" \
  --format ID,Phase,Cause,Created,Updated
export BB_EXPECTED_DEPLOYMENT_ID=<deployment-id-for-the-revert-commit>
BB_API_TOKEN="$BB_API_TOKEN" ./scripts/production-ops-drill.sh \
  --remote --url "$BB_URL" --do-app-id "$BB_DO_APP_ID" \
  --expected-deployment-id "$BB_EXPECTED_DEPLOYMENT_ID"
```

This is a rebuild, not an instant release pin. Pause new dispatch while it is in
progress and keep the incident open until the replacement deployment is
`ACTIVE` and the remote smoke passes.

If rollback does not restore `/health` and bearer read APIs, stop dispatching
new work and inspect App Platform logs:

```sh
doctl apps logs "$BB_DO_APP_ID" plane --type run --tail 200
doctl apps get "$BB_DO_APP_ID" \
  --format ID,Spec.Name,DefaultIngress,ActiveDeployment.ID,InProgressDeployment.ID
```

If logs contain `newer than this bb binary supports`, the rollback binary is
correctly refusing a newer ledger. Roll forward or restore a compatible backup;
do not force the schema version downward.

## Restart Recovery

After a restart or deploy, classify inherited running rows:

```sh
doctl apps console "$BB_DO_APP_ID" plane
# In the ephemeral console:
BB_PLANE_DIR=${BB_PLANE_DIR:-/app/plane} bb recover --json
```

Then read status from outside the instance:

```sh
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
doctl apps console "$BB_DO_APP_ID" plane
# In the ephemeral console:
litestream restore -config /tmp/bb-litestream.yml -dry-run -json /app/plane/.bb/plane.db
BB_PLANE_DIR=/app/plane bb status --json
```

A sudden container loss can lose writes since the last successful replication;
the observed heartbeat and configured `rpo_seconds` are the only bound. Treat a
stale heartbeat as unprotected, and pause dispatch before planned replacements
until `backup.status == "fresh"`.

For an operator-held proof, restore the replica to a new local destination with
an operator-local Litestream config, then verify that copy before it is trusted.
Do not copy `/app/plane/.bb/plane.db` out of a live ephemeral container and call
that the durable backup:

```sh
mkdir -p .ops/backups
backup=.ops/backups/plane-$(date +%Y%m%dT%H%M%SZ).db
litestream restore -config "$BB_LITESTREAM_CONFIG_LOCAL" \
  -o "$backup" /app/plane/.bb/plane.db
python3 - "$backup" <<'PY'
import sqlite3
import sys
db = sys.argv[1]
conn = sqlite3.connect(db)
print(conn.execute("pragma integrity_check").fetchone()[0])
print(conn.execute("select count(*) from runs").fetchone()[0])
PY
```

Restore is a manual incident action. App Platform has no durable disk to patch
in place: select a compatible Litestream replica, make it the configured restore
source, and start a fresh deployment so the entrypoint restores before `bb`
serves traffic. Verify provider deployment phase, `backup.status`, run history,
and authenticated reads before re-enabling dispatch. Never delete or rewrite a
live replica as part of a speculative rollback.

The local restore drill in `./scripts/production-ops-drill.sh --local` proves
the backup mechanism preserves run history without touching production, and it
asserts the backup readiness field through `/api/status`. It also opens a
fixture submission and checks `bb check`, `bb status --json`, `bb runs list
--json`, and `bb gate --change ops-drill --json` against the restored DB copy.

## Secret Rotation

Runtime secrets live in the private App Platform spec, not git. Rotate one value
in that private file, validate the complete spec, apply it, and wait for the new
deployment:

```sh
doctl apps spec validate "$BB_DO_APP_SPEC"
doctl apps update "$BB_DO_APP_ID" --spec "$BB_DO_APP_SPEC" --wait
doctl apps list-deployments "$BB_DO_APP_ID" \
  --format ID,Phase,Cause,Created,Updated
export BB_EXPECTED_DEPLOYMENT_ID=<deployment-id-created-by-this-update>
BB_API_TOKEN="$BB_API_TOKEN" ./scripts/production-ops-drill.sh \
  --remote --url "$BB_URL" --do-app-id "$BB_DO_APP_ID" \
  --expected-deployment-id "$BB_EXPECTED_DEPLOYMENT_ID"
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

> Note: a reflex webhook task's `[admission] attention_debt` policy defaults to
> `global` — any open dead-letter/awaiting-recovery run anywhere on the plane
> then 429s that task's deliveries. Set `attention_debt = "task"` on advisory
> tasks (e.g. `review`) so unrelated operational debt can't silently mute them
> (bitterblossom-977).
