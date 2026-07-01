# Epic: ledger durability and restore drills

Priority: P0 | Status: ready | Estimate: L

## Goal

Make BB run history, costs, submissions, DLQs, and recovery evidence survive
Fly volume loss. The ledger is the system of record for the whole factory; losing
it must be a drilled incident, not silent amnesia.

## Oracle

- [ ] Production ledger replication is configured with Litestream or an equally
      boring SQLite replication path, with secrets declared by name only.
- [ ] RPO and RTO are stated in docs and visible in an ops health/readiness
      surface.
- [ ] A restore drill recreates a plane ledger from backup into a fresh volume or
      local fixture and proves `bb check`, `bb status --json`, `bb runs list`, and
      `bb gate` still work.
- [ ] Deploy/rollback docs cover old-binary/new-schema behavior and the recovery
      command sequence after a rollback.
- [ ] CI or a local drill verifies the backup configuration without needing
      production secrets.
- [ ] `./scripts/verify.sh` passes.

## Children

- [ ] Litestream config, container wiring, and Fly volume path integration.
- [ ] Backup health/readiness output for last successful replicate timestamp.
- [ ] Restore drill script against a copied or fixture SQLite DB.
- [ ] Schema migration/rollback contract for forward-only and rollback-safe
      releases.
- [ ] Operations docs update with exact backup, restore, and rollback commands.

## Notes

The groom report flagged this as a fleet-doctrine violation: one Fly volume holds
the current ledger. Canary already carries the pattern to copy: SQLite WAL plus
replication plus a restore drill.
