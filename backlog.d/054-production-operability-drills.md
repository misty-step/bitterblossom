# Make production operation boring with restore, canary, and deploy drills

Priority: P1 | Status: ready | Estimate: L

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
