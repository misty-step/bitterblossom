# Factory Audit Report

## Summary

- Date: 2026-03-16
- Run ID: N/A (preflight failure — no run launched)
- Issue: N/A (no `autopilot` issues exist)
- PR: N/A
- Worker: N/A
- Reviewers: N/A
- Terminal State: **preflight failure**

## Timeline

| Time | Event | Notes |
|------|-------|-------|
| 2026-03-16 | preflight started | Loaded GH token, checked issues, checked workers |
| 2026-03-16 | preflight failed | 5 blocking issues prevent any run |

## Findings

### Finding 1: Elixir/OTP not installed — conductor cannot run

- Severity: P0
- Existing issue or new issue: New — no existing issue tracks this
- Observed: `mix`, `elixir`, and `erlang` are all absent from the machine. `brew list elixir` and `brew list erlang` both fail. The conductor (`conductor/`) is an Elixir project and cannot compile or execute.
- Expected: Elixir 1.16+ and Erlang/OTP installed and on PATH.
- Why it matters: The conductor is the single authority for lease, dispatch, governance, merge, and retro. Without it, the factory cannot operate. This is the most fundamental preflight failure possible.
- Evidence: `which mix` → "not found", `brew list elixir` → "No such keg"

### Finding 2: No `.env.bb` environment file

- Severity: P1
- Existing issue or new issue: New — partially related to #532 (auth interop) but distinct
- Observed: `.env.bb` does not exist. The factory-audit skill and watchpoints reference `source .env.bb` as a required preflight step.
- Expected: `.env.bb` with worker addresses, repo targets, and operational config.
- Why it matters: Without it, dispatch has no configured target. GH token was recoverable via `gh auth token`, but worker config is not.
- Evidence: `cat .env.bb` → "No such file or directory"

### Finding 3: No `autopilot` labeled issues in backlog

- Severity: P1
- Existing issue or new issue: New
- Observed: `gh issue list --label autopilot` returns empty. The factory-audit skill says to "prefer the highest-priority open autopilot issue." None exist.
- Expected: At least one issue labeled `autopilot` for the conductor to select.
- Why it matters: The conductor's issue selection depends on this label. Without it, even a working conductor would have nothing to dispatch.
- Evidence: `gh issue list --label autopilot --state open` → `[]`

### Finding 4: Worker fleet unhealthy — no workers ready for dispatch

- Severity: P1
- Existing issue or new issue: Related to #532 (auth), #680 (repo serving)
- Observed: `bb status` shows 3 workers: bb-builder (unreachable), bb-fixer (no GH auth), bb-polisher (no GH auth). WORKFLOW.md references workers by new names (moss, bramble, thorn, willow, fern, foxglove) that don't appear in `bb status`.
- Expected: At least one worker reachable, GH-authed, and idle.
- Why it matters: Even with a working conductor and autopilot issues, no worker can accept a dispatch.
- Evidence: `bb status` output shows all workers either unreachable or missing GH auth.

### Finding 5: WORKFLOW.md worker names don't match `bb status` fleet

- Severity: P2
- Existing issue or new issue: Related to #687 (rename sprites)
- Observed: WORKFLOW.md defines workers as `moss`, `bramble`, `thorn`, `willow`, `fern`, `foxglove`. `bb status` shows `bb-builder`, `bb-fixer`, `bb-polisher`. These are completely disjoint sets.
- Expected: Worker names in WORKFLOW.md match actual deployed fleet.
- Why it matters: Operator confusion. The contract says one thing, reality says another. Any automation reading WORKFLOW.md will target nonexistent sprites.
- Evidence: WORKFLOW.md `workers:` block vs `bb status` output.

### Finding 6: `scripts/conductor.py` referenced by factory-audit skill doesn't exist

- Severity: P2
- Existing issue or new issue: New
- Observed: The factory-audit skill says to launch via `scripts/conductor.py run-once ...`. No such file exists. The conductor is Elixir (`mix conductor run-once`), not Python.
- Expected: Skill references match actual entry points.
- Why it matters: The audit skill's own instructions are stale. An operator following them hits a dead end.
- Evidence: `ls scripts/conductor.py` → "No such file or directory"

## Backlog Actions

- New issues filed:
  - #694 — [P0] Elixir/OTP not installed
  - #695 — [P1] No .env.bb operational config
  - #696 — [P1] No autopilot-labeled issues
- Existing issues commented:
  - #687 — sprite rename contract/reality divergence evidence
  - #532 — auth layer broken across entire fleet
- Priority changes: None

## Reflection

- What Bitterblossom did well: `bb` builds cleanly from source. `bb status` gives a clear fleet picture. WORKFLOW.md is a well-structured contract.
- What felt brittle: The gap between WORKFLOW.md's declared architecture and the actual runtime is enormous. The factory describes a 6-worker Elixir-orchestrated pipeline, but the machine has neither Elixir nor matching workers.
- What should be simpler next time: A single `bb preflight` command that validates all prerequisites (Elixir installed, .env.bb exists, at least one healthy worker, autopilot issues available) before any run attempt. This would take the 6 findings above and surface them in 10 seconds.
