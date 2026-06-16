# Make production operation boring with restore, canary, and deploy drills

Priority: P1 | Status: done | Estimate: L

## Goal

Turn the Fly plane from a documented deployment into an operable service with
repeatable backup, restore, deploy smoke, rollback, and recovery drills.

## Oracle

- [ ] `docs/operations/` contains executable runbooks for deploy, rollback,
      restart recovery, backup, restore, secret rotation, and stuck-run
      triage.
- [ ] A script or documented command sequence performs a production-safe smoke:
      `/health`, authenticated `/api/tasks`, authenticated `/api/runs`, and
      `bb recover` inside the machine.
- [ ] A backup/restore drill proves the SQLite ledger can be restored from the
      Fly volume path without losing run history.
- [ ] CI or a release workflow names the deploy smoke gate, even if production
      credentials remain manual.
- [ ] `./scripts/verify.sh` remains the local gate and the runbook states what
      extra live evidence is required for deploys.

## Children

1. Document the Fly state and secret inventory.
2. Add backup and restore recipes for `plane/.bb/plane.db`.
3. Add a production smoke checklist/script that is safe to run read-only.
4. Add rollback and stuck-recovery runbooks.
5. Wire the runbook into `CLAUDE.md` verification guidance.

## Notes

Why: the ops lane found volume-backed state but no first-class restore or
deploy-canary contract.

Evidence:

- `fly.toml:11-14` mounts `bb_plane_data` at `/app/plane/.bb`.
- `.github/workflows/ci.yml:17-20` runs the local `./scripts/verify.sh` gate
  only.
- `docs/spine.md:224-228` names post-restart checks but not backup, restore, or
  deploy smoke as a maintained operational drill.

## Dogfood synthesis 2026-06-16

This delivery run must fold BB operator feedback into the work, not leave it in
chat:

- **Good:** `bb gate --submission <id> --json` is a useful ship oracle. It
  gives a durable verdict, member run ids, costs, and findings without relying
  on chat memory.
- **Bad:** submission storms are easy to launch without `GH_TOKEN`. The task
  specs know the required secret, but the failure is discovered only after each
  member run starts, creating multiple dead letters.
- **Ugly:** there is no first-class command to acknowledge a superseded
  pre-execute DLQ. Five no-`GH_TOKEN` failures from a superseded PR #858 storm
  remain open even after a clean replacement submission passed.

Delivery requirement added from this dogfood:

- The operations runbook and smoke drill must include a preflight step for
  declared operator secrets before fanout.
- The stuck-run/DLQ triage runbook must explicitly distinguish replayable
  pre-execute failures, superseded dead letters, and at/after-execute recovery.
- If DLQ acknowledgement is not implemented in this slice, emit a concrete
  backlog follow-up rather than treating the residual noise as "done."

## Delivery notes 2026-06-16

Delivered the first production-operability slice:

- Added `docs/operations/README.md` with executable runbooks for preflight,
  deploy, rollback, restart recovery, backup, restore, secret rotation, and
  stuck-run/DLQ triage.
- Added `scripts/production-ops-drill.sh`:
  - `--local` creates a temporary dev plane, runs one local workload, starts
    `bb serve`, checks unauthenticated `/health`, checks bearer read APIs, runs
    `bb recover --json`, and proves SQLite backup/restore preserves run
    history.
  - `--remote` checks the Fly URL read-only with bearer auth via curl config on
    stdin and runs Fly status, volume, and in-machine recovery checks when
    `flyctl` is available.
- Wired the local operations drill into `./scripts/verify.sh`, so CI exercises
  the deploy-smoke/restore proof surface indirectly through the canonical repo
  gate.
- Updated `docs/spine.md` to point operators at `docs/operations/` and the
  maintained smoke drill.
- Updated the portable `skills/bitterblossom` operator recipe so
  token-bearing curl calls use stdin config rather than exposing
  `BB_API_TOKEN` in argv.
- Added `backlog.d/064-secret-preflight-and-dlq-acknowledgement.md` for the
  first-class secret preflight and DLQ acknowledgement work discovered during
  dogfood.

Verification:

- `./scripts/production-ops-drill.sh --local` passed:
  `/health`, bearer `/api/tasks`, `/api/runs`, `/api/status`, HTML, and
  backup/restore integrity with `runs=1`.
- `cargo test --test cli_contract_docs -- --nocapture` passed.
- `./scripts/verify.sh` passed with `src LOC: 4981`.
