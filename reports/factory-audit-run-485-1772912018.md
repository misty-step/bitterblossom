# Factory Audit Report

## Summary

- Date: 2026-03-07
- Run ID: `run-485-1772912018`
- Issue: `#485` `[P1] Security: harden conductor prompts against issue-body prompt injection`
- PR: `#495` `https://github.com/misty-step/bitterblossom/pull/495`
- Worker: `pr83-e2e2-20260306-001`
- Reviewers: `council-fern-20260306`, `council-sage-20260306`, `council-thorn-20260306`
- Terminal State: conductor marked the run failed, but the builder had already produced a valid handoff and draft PR

## Timeline

| Time | Event | Notes |
|------|-------|-------|
| 2026-03-07T19:33:38Z | run start | `lease_acquired` for issue `#485` |
| 2026-03-07T19:33:42Z | reviewers ready | all three reviewers passed readiness after forced re-setup preflight |
| 2026-03-07T19:33:43Z | builder selected | worker `pr83-e2e2-20260306-001` |
| 2026-03-07T19:33:43Z to 2026-03-07T19:38:19Z | observability gap | run ledger stayed frozen in `phase=building` with no new events while real work happened on-sprite |
| ~2026-03-07T19:36:30Z | builder progress observed | worker had switched to branch `factory/485-p1-security-harden-conductor-pro-1772912018` and was editing/testing |
| 2026-03-07T19:38:04Z | PR opened | draft PR `#495` created |
| 2026-03-07T19:38:19Z | terminal failure recorded | conductor emitted `command_failed` on `bb kill ... use of closed network connection` |
| 2026-03-07T19:38:19Z | issue surface updated | issue `#485` received a failure comment even though builder artifact and PR existed |
| 2026-03-07T19:38:44Z | PR CI mostly green | CI checks passed on PR `#495`; `merge-gate` remained queued because PR stayed draft |

## Findings

### Finding: shell-dependent Python resolution breaks the conductor before any factory logic

- Severity: P2
- Existing issue or new issue: commented on `#490`
- Observed: `bash -lc` resolved `python3` to `Python 3.7.9` and `scripts/conductor.py check-env` crashed on `@dataclass(slots=True)`.
- Expected: operator startup path should fail fast with a clear interpreter/version error before any attempt to run the conductor.
- Why it matters: normal shell entrypoints are inconsistent; a supervised run required pinning `/opt/homebrew/opt/python@3.14/bin/python3.14` manually.
- Evidence: `TypeError: dataclass() got an unexpected keyword argument 'slots'`

### Finding: no clean prepared worker/reviewer pool existed before manual repair

- Severity: P1
- Existing issue or new issue: commented on `#469`
- Observed: the only reachable prepared builder had `M .dispatch-prompt.md`; another reachable builder was dirty on an old feature branch; two reviewers were dirty and the third reviewer was not prepared until `bb setup --force`.
- Expected: supervised runs should start from isolated, attributable execution surfaces without force-reprovisioning the pool first.
- Why it matters: stale workspace state raises attribution risk and increases operator toil before every deliberate run.
- Evidence: preflight `bb status <sprite>` output for `pr83-e2e2-20260306-001`, `pr83-e2e3-20260306-001`, `council-fern-20260306`, `council-sage-20260306`, `council-thorn-20260306`

### Finding: run-centric observability hid real progress for most of the builder phase

- Severity: P2
- Existing issue or new issue: commented on `#98`
- Observed: `show-runs` and the snapshot collector stayed frozen at `builder_selected` from `19:33:43Z` until the terminal failure, while the worker had already switched branches, edited files, run tests, pushed, and opened PR `#495`.
- Expected: the run surface should expose heartbeat/progress recency and meaningful intermediate state during long builder work.
- Why it matters: I had to inspect worker branch state, artifact paths, and `ralph.log` directly to distinguish “slow but healthy” from “hung.”
- Evidence: snapshot output vs on-sprite checks and `ralph.log`

### Finding: post-artifact cleanup turned a valid builder handoff into a false failed run

- Severity: P1
- Existing issue or new issue: new issue `#496`
- Observed: the worker wrote `.bb/conductor/run-485-1772912018/builder-result.json` with draft PR `#495`, but the conductor recorded `phase=failed`, `status=failed`, `pr_number=null` because `bb kill pr83-e2e2-20260306-001` failed with `use of closed network connection`.
- Expected: once artifact discovery and PR verification succeed, later cleanup noise should not overwrite that truth.
- Why it matters: the issue surface, run ledger, and PR surface disagreed about what actually happened; delivery had happened up to ready-for-review, but the control plane reported failure.
- Evidence: `builder-result.json`, `show-runs`, `show-events`, issue `#485` comments, PR `#495` state

### Finding: failed run left a live lease behind instead of clean terminal lease state

- Severity: P1
- Existing issue or new issue: commented on `#468`
- Observed: after the run had already terminated as failed, the local leases table still showed issue `#485` with future `lease_expires_at=2026-03-07T20:23:43Z` and `blocked_at=null`.
- Expected: terminal failure should release or explicitly block the lease truthfully.
- Why it matters: even without success, a dead run can strand intake until TTL expiry.
- Evidence: local SQLite query against `.bb/conductor.db`

## Backlog Actions

- New issues: `#496`
- Existing issues commented: `#490`, `#469`, `#98`, `#468`
- Priority changes: none

## Reflection

- What Bitterblossom did well: builder execution itself completed the implementation, opened draft PR `#495`, and produced a structured artifact with passing tests.
- What felt brittle: operator startup depended on shell-specific interpreter resolution; worker pool cleanliness required force setup; run progress was opaque; cleanup errors could erase truthful builder state.
- What should be simpler next time: a deliberate run should need one command, one truthful run surface, and zero sprite forensics unless the code under test is actually broken.
