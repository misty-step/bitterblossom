# Epic: ledger durability and restore drills

Priority: P0 | Status: ready | Estimate: L

## Goal

Make BB run history, costs, submissions, DLQs, and recovery evidence survive
Fly volume loss. The ledger is the system of record for the whole factory; losing
it must be a drilled incident, not silent amnesia.

## Oracle

- [x] Production ledger replication is configured with Litestream or an equally
      boring SQLite replication path, with secrets declared by name only.
- [x] RPO and RTO are stated in docs and visible in an ops health/readiness
      surface.
- [x] A restore drill recreates a plane ledger from backup into a fresh volume or
      local fixture and proves `bb check`, `bb status --json`, `bb runs list`, and
      `bb gate` still work.
- [x] Deploy/rollback docs cover old-binary/new-schema behavior and the recovery
      command sequence after a rollback.
- [x] CI or a local drill verifies the backup configuration without needing
      production secrets.
- [x] `./scripts/verify.sh` passes.

## Children

- [x] Litestream config, container wiring, and Fly volume path integration.
- [x] Backup health/readiness output for last successful replicate timestamp.
- [x] Restore drill script against a copied or fixture SQLite DB.
- [x] Schema migration/rollback contract for forward-only and rollback-safe
      releases.
- [x] Operations docs update with exact backup, restore, and rollback commands.

## Notes

The groom report flagged this as a fleet-doctrine violation: one Fly volume holds
the current ledger. Canary already carries the pattern to copy: SQLite WAL plus
replication plus a restore drill.

2026-07-02 slice: added `[backup]` runtime config for provider, replica secret
env name, heartbeat path, RPO, and RTO; `bb check --json` projects the declared
policy and `bb status --json` reports `backup.status`/`backup.healthy` from the
heartbeat without reading secret values. The local production ops drill now
sets a no-secret Litestream-shaped config, writes a heartbeat, asserts
`backup.status == "fresh"` through `/api/status`, copies the SQLite ledger into
a restored fixture DB, and proves `bb check`, `bb status --json`, `bb runs list
--json`, and `bb gate --change ops-drill --json` against the restored copy.
Verification: `./scripts/verify.sh` passed with `src LOC: 10017` under the
raised 10100 mechanism tripwire. Remaining: production Litestream/container
wiring and schema rollback contract.

2026-07-02 Litestream runtime slice: the production image now installs pinned
Litestream v0.5.13 for Linux amd64/arm64 and starts through
`bb-litestream-entrypoint`. Fly declares `BB_LITESTREAM_REQUIRED=1`, the
volume-backed DB path `/app/plane/.bb/plane.db`, the temporary config path, the
replica secret env name `LITESTREAM_REPLICA_URL`, and the heartbeat path without
storing a replica URL in git. The entrypoint writes Litestream v0.5 `replica:`
config with `${LITESTREAM_REPLICA_URL}` expansion, starts
`litestream replicate -config`, waits for the initial `litestream sync -wait`
before starting `bb serve`, writes `.bb/backup-last-success` only after sync,
fails closed when the required secret is missing, and exits the app if
Litestream dies. Added a fake-Litestream Rust integration test that proves
env-name-only config generation, no secret leakage, and heartbeat creation
without production credentials. Remaining: schema migration/rollback contract
for old-binary/new-schema releases.

2026-07-02 schema rollback slice: `bb` now stamps the SQLite ledger with
`PRAGMA user_version = 1`, exposes `ledger.schema_version` and
`ledger.supported_schema_version` in `bb status --json` and `bb check --json`,
and refuses to open a ledger whose user_version is newer than the binary
supports before running migrations. The rollback docs now require checking that
schema version before rollback, running `bb recover --json` after rollback, and
rolling forward or restoring a compatible backup instead of forcing
`PRAGMA user_version` downward. This completes the 090 oracle.
