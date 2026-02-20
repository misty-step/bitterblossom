---
name: e2e-test
user-invocable: true
description: "End-to-end shakedown of the bb dispatch pipeline. Exercises build, fleet health, credential validation, dispatch, monitoring, and completion against a real sprite and issue. Acts as adversarial QA — documents every friction point."
allowed-tools:
  - Read
  - Grep
  - Glob
  - Bash
---

# E2E Dispatch Shakedown

Run this skill to exercise the full `bb dispatch` pipeline against a real issue. You are an adversarial QA tester — document every friction point, not just failures.

## Key Principles

- **Adversarial mindset.** Assume something is broken. Prove it isn't.
- **Timestamp everything.** Record wall-clock time at each phase boundary.
- **Known failure modes.** Watch for: stale TASK_COMPLETE (#277), polling loops (#293), zero-effect oneshot (#294), proxy health failures (#296), stdout pollution (#320).
- **Friction counts.** A confusing message or 90-second silence is a finding, not "working as expected."
- **Credential safety.** Never enumerate local keychains or brute-force credential stores. If auth is missing/broken, mark FAIL + file issue; do not probe secrets.

## References

Read before starting:
- `references/evaluation-criteria.md` — per-phase PASS/FRICTION/FAIL rubric
- `references/friction-taxonomy.md` — F1-F9 categories with severity floors

## Workflow

### Phase 1: Build

```bash
go build -o ./bin/bb ./cmd/bb
```

Record: exit code, duration, any warnings.

### Phase 2: Fleet Health

```bash
source .env.bb
bb status
```

Pick a sprite showing `warm` API status. Record which sprite and its state.

### Phase 3: Issue Selection

Select an issue from the backlog suitable for dispatch. Fetch full context locally — sprites cannot access GitHub issues.

```bash
gh issue view <NUMBER> --repo misty-step/bitterblossom
```

Embed the issue body verbatim in the dispatch prompt. Do not use `--issue`.

### Phase 4: Credential Validation

```bash
export GITHUB_TOKEN="$(gh auth token)"
bb dispatch <sprite> "test" --repo misty-step/bitterblossom
```

Dry-run confirms credentials resolve. Record any validation errors or warnings.

### Phase 5: Dispatch

```bash
bb dispatch <sprite> "<prompt with embedded issue context>" \
  --repo misty-step/bitterblossom \
  --timeout 25m
```

Record: dispatch confirmation output, any immediate errors.

### Phase 6: Monitor

While `--wait` is active, observe:
- Progress messages (expect every ~30s)
- Stderr vs stdout separation (--json mode)
- Any silence gaps >60s

If wait appears stalled:

```bash
bb status <sprite>
bb logs <sprite> --follow --lines 100
bb kill <sprite>
```

### Phase 7: Completion

Record: exit code, signal file state, duration. Check whether exit code reflects actual outcome (#298).

If dispatch produced a PR:

```bash
gh pr list --repo misty-step/bitterblossom --author @me --state open
```

### Phase 8: PR Quality (if applicable)

Review the PR for: meaningful commits, passing CI, correct branch target, clean diff.

### Phase 9: Findings Report & Issue Filing

Write the report using `templates/findings-report.md`. Include every friction point, not just failures.

**Issue filing is mandatory for failed runs.** If any phase scored FRICTION or FAIL:

1. File a GitHub issue for every finding P0-P2. Use the friction taxonomy category and severity from the report.
2. Comment on existing issues when a finding confirms a known bug (don't duplicate).
3. Update the report with issue numbers after filing.
4. Update `MEMORY.md` filed issues section with new issue numbers.

```bash
gh issue create --repo misty-step/bitterblossom \
  --title "fix: <finding title>" \
  --label "bug,area/dispatch,<priority>" \
  --body "<context, problem, expected, impact, files, acceptance criteria>"
```

The report alone is not the deliverable — **filed issues are**. A shakedown that finds problems but doesn't file issues is incomplete.
