# Bitterblossom operations

This is the canonical operations contract for the shipped Bitterblossom
service. Production runs on the operator's machine as a launchd-managed,
local-primary plane. Hosted deployment notes are historical reference only and
are not an active authority.

## Production contract

The service is the launchd job com.misty-step.bb-serve and starts the tracked
entrypoint with the atomically installed user-owned binary at
`~/.local/libexec/bitterblossom/bb --config plane serve`. Litestream runs as the
separate com.misty-step.bb-plane-litestream sidecar. The installer stages
`target/release/bb` and never asks launchd to execute from the build tree.

- **Bind:** 127.0.0.1:7093; /health is the unauthenticated liveness probe.
- **Mode:** dev = false and allow_local_substrate = true. The explicit grant
  is required because local execution runs with the operator's account. It is
  an intentional trust decision, not a dev shortcut.
- **Data:** plane/.bb/plane.db, with SQLite WAL enabled. The database is the
  system of record; never edit it with a text editor or lower PRAGMA user_version.
- **Backup:** [backup] uses Litestream and the named
  LITESTREAM_REPLICA_URL environment variable. The heartbeat at
  plane/.bb/backup-last-success is the freshness receipt. The URL and all
  other values remain in the operator-local launchd environment, never in git,
  plane config, argv, or logs.
- **Credentials:** Mint supplies scoped service credentials. Launchd injects
  only the declared BB_API_TOKEN, webhook/HMAC values, provider key, GitHub
  tokens, SPRITE_TOKEN for the bounded alternate, Canary check-in values, and
  LITESTREAM_REPLICA_URL. Inspect names with bb preflight and bb doctor;
  never print secret values.
- **Logs:** launchd writes stdout/stderr under
  ~/.local/state/bitterblossom/bb-serve.out.log and
  ~/.local/state/bitterblossom/bb-serve.err.log. The Litestream supervisor
  has separate logs beside them.
- **Alternate substrate:** sprites and tailnet remain configured alternates.
  They are selected by task/workflow config when isolation is required; they do
  not change the local-primary service contract.

The live config readback must show these values before enabling unattended PR or
merge loops:

    dev = false
    allow_local_substrate = true
    db_path = ".bb/plane.db"

    [ingress]
    bind = "127.0.0.1:7093"

    [backup]
    enabled = true
    provider = "litestream"
    replica_env = "LITESTREAM_REPLICA_URL"
    last_success_path = ".bb/backup-last-success"

## Readiness and safe readback

Run from the Bitterblossom checkout. The primary drill reads config, health,
status, runs, and DLQ state. It does not run agents, replay or acknowledge a
DLQ, alter the ledger, or restart launchd. `bb doctor --expect-serve` is a
separate config/database/preflight/health check; doctor reports exit 2 for a
failed check, while only this drill owns the open-DLQ exit 3 gate:

    BB_RUNTIME_PLANE=plane BB_BIN="$HOME/.local/libexec/bitterblossom/bb" \
      ./scripts/production-ops-drill.sh --primary

The primary drill also opens SQLite read-only and checks journal_mode = wal,
PRAGMA integrity_check = ok, run history, the backup heartbeat, launchd
ownership, and filesystem headroom. It reports READINESS BLOCKED and exits
non-zero when any dead letter has derived status = "open". Replayed rows are
resolved and do not block; acknowledged rows are resolved and do not block.
Rows with a null acknowledgement timestamp are reported as historical context,
not as a second gate. Resolve the open row or explicitly acknowledge it only
after inspecting its side effects; do not mutate it as part of this drill.

The current known live residual is DLQ #29 (powder-chew, missing
POWDER_API_BASE_URL). The readback must expose it without attempting replay or
acknowledgement. Historical rows with null acknowledgement timestamps remain a
separate count until their existing replay/acknowledgement outcome is reviewed.
Do not enable PR/merge loops while status = "open" remains.

The primary drill is the read-only service check. It deliberately does not invoke
`bb doctor`, `bb check`, `bb status`, `bb runs`, `bb dlq`, `bb recover`, or `bb serve`:
those CLI paths open the ledger through the migration layer and may apply schema
maintenance. It reads `/health`, `/api/status`, `/api/runs`, and `/api/dlq` over HTTP,
then uses Python sqlite3 with `mode=ro&immutable=0`, `PRAGMA query_only = ON`,
`journal_mode`, `integrity_check`, schema, counts, and state snapshots before and
after the HTTP readback. It also parses launchctl ownership and requires backup
status=fresh, healthy=true, and age <= RPO when backup is enabled.

To inspect the unauthenticated liveness route manually:

    curl --fail --silent http://127.0.0.1:7093/health

For a protected local-primary install, keep the token in an operator-local env
file rather than a raw export or command argument. The file must be owner-only
(0600 or stricter), and the drill sources it silently without printing values:

    BB_ENV_FILE="$HOME/.config/bitterblossom/primary.env" \
      BB_RUNTIME_PLANE=plane BB_BIN="$HOME/.local/libexec/bitterblossom/bb" \
      ./scripts/production-ops-drill.sh --primary

When no token is configured, the drill accepts API 200 only because the service
binds to loopback. When a token is configured, it proves no-header 401 and
bearer-authenticated 200 before reading status/runs/DLQ. Never put a token in
argv, a query string, logs, or evidence.

## Startup, drain, and restart

Install or update the reproducible release services before first start:

    cargo build --release
    ./scripts/install-bb-local-primary.sh

The installer atomically stages `target/release/bb` into
`~/.local/libexec/bitterblossom/bb`, renders the tracked launchd templates with
that stable path, verifies `dev = false`, `allow_local_substrate = true`, and
`[ingress] bind = "127.0.0.1:7093"`, then loads/kickstarts the serve and
Litestream labels. A failed copy leaves the previous installed binary intact.
It never writes credentials or plane state. If the retired
`com.misty-step.bb-dashboard` plist is detected, rerun with
`--retire-legacy-dashboard` to explicitly unload and remove it; normal install
does not delete it.

1. Check the launchd job and logs without changing it:

       launchctl print gui/$(id -u)/com.misty-step.bb-serve
       tail -n 100 ~/.local/state/bitterblossom/bb-serve.err.log

2. Run `scripts/production-ops-drill.sh --primary`. Record the configured bind,
   schema, queue counts, backup freshness, and DLQ status. Do not run a mutating
   CLI as part of this read-only receipt.

3. For a planned restart, stop new admission at the owning operator workflow,
   then send SIGTERM. bb serve polls for SIGTERM and exits normally after its
   current request loop; it does not blind-replay inherited work:

       launchctl kill SIGTERM gui/$(id -u)/com.misty-step.bb-serve
       tail -n 50 ~/.local/state/bitterblossom/bb-serve.err.log
       launchctl kickstart -k gui/$(id -u)/com.misty-step.bb-serve

4. Re-run `scripts/production-ops-drill.sh --primary`. Run `bb recover --json`
   only after inspecting side effects of inherited rows;
   unknown probes retain their host lease. Resolve awaiting_recovery explicitly,
   never by blindly replaying an agent execution.

5. If health does not return on 127.0.0.1:7093, stop the PR/merge loop, capture
   the launchd error log and failed primary receipt, and follow rollback below.

The throwaway proof of SIGTERM drain and restart is separate from production:

    BB_BIN=./target/debug/bb ./scripts/production-ops-drill.sh --dev-temp

## Persistence, backup, and restore

Before any destructive repair, capture the read-only primary receipt. Do not use
`bb` CLI commands here because ledger open may apply migrations:

    BB_RUNTIME_PLANE=plane BB_BIN="$HOME/.local/libexec/bitterblossom/bb" \
      ./scripts/production-ops-drill.sh --primary

The production drill performs the SQLite readback and checks the backup
heartbeat. The dev-temp drill performs one isolated SQLite backup and restore,
then checks integrity and that run history is unchanged. A production restore
must use a new isolated directory first; never overwrite plane/.bb/plane.db
in a rehearsal:

    BB_BIN=./target/debug/bb ./scripts/production-ops-drill.sh --dev-temp

For an actual incident restore, stop launchd, preserve the failed database and
WAL files, restore into plane-restore/, run the isolated `--dev-temp` read
surface against that directory, compare schema and run counts, then seek
operator approval before swapping the live data path. Keep the previous binary,
config, and ledger as rollback artifacts until the replacement passes readback.

## Resource headroom and rollback

Before enabling unattended work, record:

    df -h plane/.bb
    du -sh plane/.bb ~/.local/state/bitterblossom
    launchctl print gui/$(id -u)/com.misty-step.bb-serve | sed -n '1,120p'

Leave enough disk for the SQLite WAL, Litestream queue, logs, and one isolated
restore. If the binary or config is bad:

1. disable PR/merge admission;
2. capture the primary HTTP/store readback, DLQ, logs, and the preserved database/WAL;
3. stop and drain launchd;
4. restore the last known-good binary/config pair without editing schema
   metadata;
5. kickstart the same com.misty-step.bb-serve job;
6. rerun the full readback and --dev-temp restore rehearsal before reopening
   admission.

Rollback is a deliberate operator action. It never replays agent work and never
silently discards an open DLQ.

## Gate and historical reference

The repository gate runs the deterministic throwaway drill with --dev-temp.
The local-primary drill is an operator-supplied live readback and must be run
before a production change is marked ready:

    ./scripts/verify.sh
    BB_RUNTIME_PLANE=plane BB_BIN="$HOME/.local/libexec/bitterblossom/bb" \
      ./scripts/production-ops-drill.sh --primary

The retired hosted-plane runbook is preserved at
 docs/archive/operations/hosted-app-platform-reference.md
and is reference-only. It must not be linked as current deployment authority
or used to select a production host.
