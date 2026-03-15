# Factory Audit Report — Always-On Service First Run

## Summary

- Date: 2026-03-15
- Mode: Always-on service (`mix conductor start`)
- Fleet: 3 sprites (bb-builder, bb-fixer, bb-polisher) from fleet.toml
- Harness: Codex gpt-5.4
- Issues processed: #624 (merged), #622 (merged after retry)
- PRs: #672 (merged), #673 (merged)
- Terminal State: Both issues delivered end-to-end

## Timeline

| Time (UTC) | Event |
|------------|-------|
| 11:59:43 | `mix conductor start` — 3 sprites healthy, all services running |
| 11:59:44 | Expired 2 stale leases from prior runs |
| 11:59:47 | Builder dispatched for #622 and #624 (concurrent on bb-builder) |
| 12:06:57 | **PR #672** (issue #624) — polisher picks up (CI green) |
| 12:09:50 | **PR #672 merged** — lgtm + green CI |
| 12:09:54 | Self-update: conductor hot-reloads after merging its own code |
| 12:11:52 | Run #624 artifact read — builder reports ready |
| 12:15:59 | **PR #673** (issue #622) — polisher picks up (CI green) |
| 12:17:51 | **Run #622 failed**: artifact_missing (builder-result.json not found) |
| 12:18:02 | Conductor retries #622 — adopts existing PR #673, re-dispatches builder |
| 12:20:04 | **PR #673 merged** — lgtm + green CI |
| 12:20:07 | Self-update: conductor hot-reloads again |
| 12:21:15 | Run #622 (retry) artifact read — builder reports ready |

**Total time**: 21 minutes from start to both issues merged.

## Full Pipeline Proof (Issue #624)

```
builder (Codex, 7 min) → PR #672 opened
  → polisher (3 min) → lgtm label added
    → merge loop → squash-merged
      → self-update → conductor hot-reloaded
```

This is the first complete builder → polisher → merge cycle running autonomously.

## Findings

### Finding 1: Artifact missing on first #622 attempt (P2)

- Severity: P2
- Observed: Builder (Codex) completed with exit 0 and opened PR #673, but didn't write `builder-result.json`
- Expected: Every successful builder dispatch writes the artifact file
- Impact: Run #622 initially failed with `artifact_missing`, but the conductor retried automatically (adopted existing PR, re-dispatched)
- Root cause: Codex doesn't know about the builder-result.json artifact convention. The builder prompt template tells Claude Code to write it, but Codex may not follow Claude-specific instructions
- Action: The builder prompt template needs to be harness-agnostic, or a post-dispatch step should create the artifact from the PR state

### Finding 2: Conductor retries re-leased already-merged issue #624 (P2)

- Severity: P2
- Observed: After PR #672 merged, the conductor tried to re-lease #624 repeatedly ("worker bb-builder busy, deferring issue #624" — 6 times)
- Expected: Once a PR is merged for an issue, the issue should not be re-leased
- Root cause: The run completed as `pr_opened`, not `merged`. The merge happened via the orchestrator's merge loop, which updated the run store. But the issue's `autopilot` label was still present, making it eligible again.
- Action: The merge loop should close the GitHub issue after merging, or the orchestrator's `list_eligible` should filter out issues with merged PRs

### Finding 3: Polisher completed after merge (P3, timing)

- Severity: P3
- Observed: For PR #672, the merge happened at 12:09:50 but the polisher "completed work" at 12:11:44 — 2 minutes after merge
- Expected: Polisher finishes before or concurrent with merge
- Root cause: The polisher dispatch is async. It was dispatched at 12:06:57, and the merge loop independently saw `lgtm` + green CI at 12:09:50 and merged. The polisher sprite was still running its Codex process when the PR merged under it.
- Impact: The polisher's work on a merged PR is wasted compute. Not a correctness issue — the merge loop doesn't wait for the polisher to finish, it just checks labels + CI.
- Action: Minor — the polisher could check if the PR is still open before starting work

### Finding 4: Retro tried to create issue with missing label (P3)

- Severity: P3
- Observed: `[retro] issue creation failed: could not add label: 'source/retro' not found`
- Expected: Retro creates issues with correct labels
- Action: Create the `source/retro` label on the repo, or make retro resilient to missing labels

### Finding 5: Self-update works perfectly (Positive)

- Observed: Both PR #672 and #673 touched conductor code. After each merge, the conductor detected this, ran `git pull`, recompiled, and hot-reloaded. Zero downtime.
- Impact: The conductor can improve itself without restart. This is exactly the cybernetic governor pattern from project.md.

### Finding 6: Parallel reconciliation worked (Positive)

- Observed: All 3 sprites reconciled in parallel — 1.2s total for the fleet
- Before: Sequential would have been ~3.6s
- Impact: Fast boot, good DX for iterating

### Finding 7: Single builder sprite is a bottleneck (P3, operational)

- Observed: Two issues dispatched concurrently to bb-builder. After one finished, "worker busy" deferred the next for 6 minutes
- Action: Consider a second builder sprite in fleet.toml for higher throughput

## Backlog Actions

| Finding | Action |
|---------|--------|
| #1: Artifact missing with Codex | File new issue: harness-agnostic builder artifact protocol |
| #2: Re-leasing merged issues | File new issue: close issue after merge or filter by merged PRs |
| #3: Polisher after merge | Low priority — note in existing orchestrator docs |
| #4: Missing source/retro label | Quick fix — create the label |
| #7: Single builder bottleneck | Operational — add sprite to fleet.toml when needed |

## Reflection

- **What Bitterblossom did well**: The always-on service works. Two P1 issues went from open to merged in 21 minutes with zero operator intervention. Fleet provisioning, builder dispatch, polisher engagement, lgtm labeling, merge, and self-update all worked end-to-end. The conductor recovered from artifact failure automatically.
- **What felt brittle**: The Codex harness doesn't know about the builder-result.json artifact convention. This worked by accident for #624 (artifact written) but failed for #622 (artifact missing). The prompt template is Claude Code-centric.
- **What should be simpler next time**: (1) The builder artifact should be written by the conductor after dispatch, not by the agent. (2) Issues should auto-close on merge. (3) The fixer never fired — both PRs had green CI on first push, which is great but means we haven't stress-tested the fixer path yet.
