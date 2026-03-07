# Factory Audit Report

## Summary

- Date: 2026-03-06
- Run ID: `run-484-1772844224`
- Issue: `#484` `[P1] Governance: do not merge while trusted external reviews are still pending`
- PR: `#489` `feat: withhold merge while trusted external review surfaces are pending`
- Worker: `pr84-gov-20260306-001`
- Reviewers: `council-fern-20260306`, `council-sage-20260306`, `council-thorn-20260306`
- Terminal State: `failed`

## Timeline

| Time | Event | Notes |
|------|-------|-------|
| 2026-03-06 18:42:53 -0600 | audit start | chose `#484`; existing builder pool was stale/dirty |
| 2026-03-06 18:43:44 -0600 | lease acquired | `run-484-1772844224` |
| 2026-03-06 18:43:46 -0600 | builder selected | `pr84-gov-20260306-001` |
| 2026-03-06 18:49:28 -0600 | PR opened | `#489` existed before run ledger surfaced it |
| 2026-03-06 18:49:36 -0600 | builder complete | artifact ingested after PR/CI already existed |
| 2026-03-06 18:50:03 -0600 | run failed | reviewer `council-thorn-20260306` repo sync failed |

## Findings

### Finding: shell-dependent Python interpreter drift

- Severity: P2
- Existing issue or new issue: `#490`
- Observed: `bash -lc` resolved `python3` to `/usr/local/bin/python3` `3.7.9`, while the normal shell resolved `python3` to `3.12.8`. A documented `run-once` command failed immediately on `@dataclass(slots=True)`.
- Expected: operator commands should validate interpreter compatibility before run start.
- Why it matters: a normal documented invocation can fail before any factory logic runs.
- Evidence: initial run attempt failed before lease progression.

### Finding: builder phase remained too opaque

- Severity: P2
- Existing issue or new issue: existing issue `#98`
- Observed: the run stayed flat at `lease_acquired` + `builder_selected` for minutes. PR `#489` existed and CI had started before `builder_complete` appeared in the run ledger.
- Expected: builder heartbeats, PR-open timestamps, and artifact-discovery timestamps should make the phase narratable.
- Why it matters: operators cannot tell “working” from “stalled” without dropping to ad hoc worker inspection.
- Evidence: `#489` existed while `show-runs` still reported `phase=building`.

### Finding: builder edited on `master` before the run branch became visible

- Severity: P1
- Existing issue or new issue: existing issue `#469`
- Observed: the fresh builder sprite showed dirty edits on `master` while implementing `#484`, even though the builder artifact later claimed branch `factory/484-p1-governance-do-not-merge-while-1772844224`.
- Expected: a run-scoped checkout should exist before edits begin.
- Why it matters: branch hygiene is part of correctness; editing on `master` first makes state attribution and recovery harder.
- Evidence: repeated `bb status` / `sprite exec` checks during builder phase.

### Finding: reviewer readiness was not validated early enough

- Severity: P1
- Existing issue or new issue: existing issue `#480`
- Observed: the run failed after builder completion because reviewer `council-thorn-20260306` could not sync its repo: `fatal: not a git repository (or any of the parent directories): .git`.
- Expected: reviewer capacity should be known-good before or at least immediately when the run enters review, with failover instead of wasting a full builder cycle.
- Why it matters: the system spent real work to open `#489`, then died on reviewer infrastructure.
- Evidence: `command_failed` event at `2026-03-07T00:50:03Z`.

### Finding: raw Ralph logs are still a poor human audit surface

- Severity: P2
- Existing issue or new issue: existing issue `#98`
- Observed: `ralph.log` is dominated by giant stream-json payload lines. Even `tail -n` can explode into unreadable output.
- Expected: run-scoped logs should be concise enough to support operator diagnosis.
- Why it matters: humans still need to inspect runs; unusable logs push them into slower, more error-prone spelunking.
- Evidence: builder log inspection during the audit.

## Backlog Actions

- New issues:
  - `#490` `Env: fail fast on unsupported Python interpreter resolution`
- Existing issues commented:
  - `#480` worker pool / reviewer readiness
  - `#469` per-run worktrees / branch isolation
  - `#98` observability / run narrative
- Priority changes:
  - none yet, but `#480` and `#469` are now even more clearly on the critical path

## Reflection

- What Bitterblossom did well:
  - fresh builder provisioning worked quickly
  - builder implemented `#484`, opened `#489`, and produced a correct artifact with tests
  - the run/event ledger still gave enough structure to reconstruct the failure
- What felt brittle:
  - interpreter resolution depends on shell
  - builder handoff remains too silent
  - reviewer readiness is not trustworthy
  - worker state attribution remains messy
- What should be simpler next time:
  - deterministic interpreter selection
  - explicit run-scoped checkout before edits
  - preflight reviewer pool health
  - first-class builder/reviewer phase heartbeats
