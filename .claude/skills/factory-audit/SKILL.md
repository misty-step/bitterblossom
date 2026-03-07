---
name: factory-audit
description: |
  Supervise one Bitterblossom conductor run as a live end-to-end test, then turn observed friction into backlog updates. Use when running a Bitterblossom issue deliberately, watching builder/reviewer/CI/merge behavior step by step, validating governance, or reflecting on whether the factory is getting better or worse. Keywords: supervised run, factory audit, conductor shakedown, trace bullet, run-once, backlog feedback, issue triage, merge policy, worker health, observability.
allowed-tools:
  - Read
  - Grep
  - Glob
  - Bash
---

# Factory Audit

Run one Bitterblossom issue as a supervised conductor exercise. Treat the run as both delivery and diagnosis: prove the issue ships, then prove the factory handled it elegantly.

## Workflow

### 1. Preflight the control plane

- Read `README.md`, `docs/CONDUCTOR.md`, and the target issue.
- Load `source .env.bb` and `export GITHUB_TOKEN="$(gh auth token)"`.
- Build `bb` if the transport changed: `go build -o ./bin/bb ./cmd/bb`.
- Pick a healthy prepared worker. If no worker is both reachable and prepared, record that as friction before fixing it.

### 2. Launch a deliberate run

- Prefer the highest-priority open `autopilot` issue unless the operator specifies one.
- Use `python3 scripts/conductor.py run-once ...` rather than `loop` so the run has a crisp boundary.
- Record the start timestamp, chosen worker, issue number, and reviewers up front.

### 3. Watch the run continuously

- Poll the run with `.claude/skills/factory-audit/scripts/collect_run_snapshot.py` instead of guessing from one surface.
- Check builder, council, PR, CI, external review, and merge policy transitions.
- Treat silence, lag, stale state, skipped reviews, dirty workspaces, or misleading output as findings even when the run eventually succeeds.

### 4. Verify the terminal state

- Confirm the GitHub issue, PR, run ledger, and latest checks all agree.
- If the PR merged, verify whether it merged at the right time, not merely whether it merged.
- If the run blocked or failed, identify whether the factory surfaced the reason cleanly and whether recovery was obvious.

### 5. Update the backlog

- Use `references/backlog-rules.md` to decide: new issue, comment on existing issue, or no action.
- File new issues for real P0-P2 findings.
- Comment on existing issues when the run provides stronger evidence, sharper scope, or revised priority.
- Do not create duplicate issues for cosmetic nits unless they reveal a broader system pattern.

### 6. Leave a durable artifact

- Write a report from `templates/factory-audit-report.md`.
- Include the run id, issue, PR, timestamps, observed friction, and the exact issue numbers filed or updated.

## Key Principles

- Separate evidence from interpretation. Capture timestamps, statuses, and artifacts before judging them.
- Delivery is not enough. A run that ships while being confusing, brittle, or overly manual is still a useful failure.
- Prefer comments on existing backlog items over duplicate issues when the problem is already known.

## References

- `references/watchpoints.md` - phase-by-phase failure patterns and what to inspect
- `references/backlog-rules.md` - when to file, comment, reprioritize, or ignore
- `templates/factory-audit-report.md` - durable report structure

## Scripts

- `./.claude/skills/factory-audit/scripts/collect_run_snapshot.py` - collect run, event, PR, thread, and issue state into one JSON snapshot
