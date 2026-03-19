# Issue 730 Single-PR Invariant Plan

> Scope issue: #730

## Goal

Keep each builder run on exactly one conductor-owned branch and make duplicate PRs on foreign branches visible and actionable before review automation can pick the wrong lane.

## Product Spec

### Problem

The builder worktree is created on a conductor-owned `factory/...` branch, but Codex can still create and push a second human-style branch such as `cx/...`. When that happens, the conductor only tracks the `factory/...` lane while downstream automation can still discover and act on the foreign PR, which breaks governance truth and wastes review cycles.

### Intent Contract

- Intent: enforce the pre-created run branch mechanically and fail closed when an extra PR for the same issue appears on a foreign branch.
- Success Conditions: builder prompts treat the branch as pre-created, the prepared workspace rejects pushes from other branches, and run completion detects duplicate open PRs tied to the same issue.
- Hard Boundaries: keep the fix inside the conductor runtime path; do not redesign PR review selection or GitHub merge policy in this lane.
- Non-Goals: solve historical duplicate PR cleanup across the whole repo, change human operator branch conventions, or add a new external service.

## Technical Design

### Approach

1. Install a per-worktree `pre-push` guard during workspace preparation and branch adoption so pushes only succeed from the expected branch and only to the matching remote ref.
2. Update the Weaver prompt to reflect the real contract: the conductor already prepared the branch, so the worker must stay on it instead of creating a fresh branch.
3. After builder completion, scan open PRs for the issue. If a non-expected branch PR is present, record the duplicate explicitly and fail the run instead of silently continuing.

### Files to Modify

- `conductor/lib/conductor/workspace.ex` — install the branch guard in prepared/adopted worktrees.
- `conductor/lib/conductor/prompt.ex` — remove the incorrect “create branch” instruction and replace it with “stay on the prepared branch”.
- `conductor/lib/conductor/run_server.ex` — detect duplicate open PRs for the leased issue before marking the run PR-ready.
- `conductor/lib/conductor/github.ex` and `conductor/lib/conductor/code_host.ex` — expose issue-scoped open PR lookup the run server can use.
- `conductor/test/conductor/*` — regression coverage for branch guarding, prompt contract, duplicate PR detection, and issue-scoped PR lookup.

### Implementation Sequence

1. Write failing tests for the prompt contract and duplicate PR detection path.
2. Add a reusable workspace branch-guard script generator and assert the generated prep commands include it.
3. Add issue-scoped PR enumeration and wire run-server duplicate detection before PR handoff.
4. Re-run focused Elixir tests, then the full conductor suite if the targeted slice is green.

### Risks & Mitigations

- Risk: a foreign branch PR created before the guard lands still confuses later automation.
  Mitigation: run-server duplicate detection fails the lane immediately and records the unexpected branch.
- Risk: the guard blocks legitimate push flows such as force pushes to the same branch.
  Mitigation: scope the hook to branch identity only; it should allow normal pushes when local and remote refs match the prepared branch.
- Risk: prompt-only changes regress later when agent defaults drift again.
  Mitigation: treat the prompt fix as clarification only and keep the guard as the actual enforcement point.
