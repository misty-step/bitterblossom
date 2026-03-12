# Issue 505 Walkthrough: QA Intake Lane

## Claim

Issue [#505](https://github.com/misty-step/bitterblossom/issues/505) now has a narrow, runnable QA-intake lane inside `scripts/conductor.py`:

- a configurable `qa-intake` command executes an external probe command that emits JSON findings
- the conductor normalizes those findings into a stable GitHub issue contract with severity, environment, repro steps, evidence, and a deterministic dedupe marker
- duplicate findings append evidence to the existing `source/qa` issue instead of creating backlog spam
- routing prefers `source/qa` issues over ordinary backlog work within the same priority tier
- workspace locking no longer depends on shell `flock`; the conductor now uses an inline Python `fcntl.flock` path that keeps the worktree tests deterministic on this branch

## Reviewer Entry Point

Start with the focused QA-and-lock coverage:

```bash
python3 -m pytest -q scripts/test_conductor.py -k 'qa or worktree or pick_issue_prefers_qa_origin'
```

Expected on this branch:

- `11 passed`
- QA finding normalization is covered
- novel-vs-duplicate GitHub sync behavior is covered
- `qa-intake` command execution is covered
- routing preference for `source/qa` is covered
- worktree prepare/cleanup locking still passes after the lock-path simplification

Then run the full conductor suite:

```bash
python3 -m pytest -q scripts/test_conductor.py
```

Expected on this branch:

- `247 passed`

## Before / After

Before:

- Bitterblossom had GitHub issue intake plus future Sentry-derived issue intake, but no explicit lane for deployed-app QA findings.
- There was no stable issue contract for QA findings, no dedupe marker, and no routing preference for QA-originated risk.
- Worktree lock coverage depended on shell `flock` behavior in tests.

After:

- `qa-intake` can run a probe command, parse structured findings, and sync them into GitHub issues or comments.
- QA-created issues carry stable hidden dedupe metadata plus the operator-visible evidence needed for autonomous follow-up.
- same-tier routing now prefers `source/qa`.
- worktree locking uses inline Python `fcntl.flock`, which keeps the serialization contract explicit and the test harness trustworthy.

## Files

- `scripts/conductor.py`
- `scripts/test_conductor.py`
- `docs/CONDUCTOR.md`

## Persistent Verification

- `python3 -m pytest -q scripts/test_conductor.py`
