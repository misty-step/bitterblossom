# Bitterblossom Operations

These runbooks are the operator contract for the Fly-hosted plane. They are
safe to paste into a terminal after replacing the environment variables; do not
put bearer tokens in URLs or command arguments that will be logged.

Canonical app and state:

```sh
export BB_FLY_APP=bitterblossom-plane
export BB_URL=https://bitterblossom-plane.fly.dev
```

The production SQLite ledger lives on the Fly volume mounted at
`/app/plane/.bb`, with the database path `/app/plane/.bb/plane.db`.

## Preflight

Before dispatching a storm or touching production, check the operator surface:

```sh
git status --short --branch --untracked-files=all
flyctl orgs list
sprite org list
sprite use -o misty-step lane-1
sprite org list
sprite exec -- whoami
./target/debug/bb --config plane check
./target/debug/bb --config plane status --json
./target/debug/bb --config plane dlq list --json
```

Before running GitHub-backed BB tasks, make `GH_TOKEN` explicit for the command
that fans out the work:

```sh
GH_TOKEN=$(gh auth token) ./target/debug/bb --config plane run verify \
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

Run the production smoke after deploy:

```sh
BB_API_TOKEN="$BB_API_TOKEN" \
  ./scripts/production-ops-drill.sh --remote --url "$BB_URL" --fly-app "$BB_FLY_APP"
```

The smoke checks unauthenticated `/health`, bearer-only read APIs, Fly app and
volume visibility, and `bb --config plane recover --json` inside the machine.

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
flyctl ssh console --app "$BB_FLY_APP" --command 'bb --config plane recover --json'
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
./target/debug/bb --config plane runs show <run-id> --json
./target/debug/bb --config plane runs resolve <run-id> success --reason "<side effects inspected>"
```

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
the backup mechanism preserves run history without touching production.

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
./target/debug/bb --config plane status --json
./target/debug/bb --config plane dlq list --json
```

Classify each problem:

- **Replayable pre-execute DLQ:** missing secret, failed clone, unavailable
  sprite before the agent executed. Fix the condition, then run
  `bb dlq replay <id> --json`.
- **Superseded pre-execute DLQ:** a replacement submission or run already
  passed. Record the DLQ id in closeout; there is no first-class acknowledge
  command yet.
- **At/after-execute uncertainty:** use `bb runs show <run-id> --json`, inspect
  artifacts and external side effects, then resolve only with
  `bb runs resolve`.

Never hide open DLQs in summaries. If they are known superseded noise, say that
explicitly and link the replacement run or submission.
