# Add a refactor dispatch workload (diff-producing, not a review lens)

Priority: P1 | Status: ready | Estimate: M

## Goal

Give the plane a first-class **refactor** workload: an operator-dispatched
agent that takes a target subsystem + structural goal and produces a
behavior-preserving refactor diff on a branch, gate-green — the sibling of
`build`, distinct from the read-only `simplification` review lens.

## Oracle

- [ ] `bb run refactor --payload '{"repo":"o/r","target":"<path>","goal":"<structural outcome>"}'`
      produces a branch with a refactor diff and a receipt (run id, cost, tokens).
- [ ] Dispatch-mode (claude/codex on subscription auth), not reflex — no API
      keys, never auto-fired.
- [ ] Behavior preservation is gated: the agent runs the repo verification gate
      and reports green (or stops and explains), per `/refactor` discipline.
- [ ] Defined entirely as config — task.toml + card.md + agent.toml — with
      **zero** workload-specific `src/` code (proves project.md's "a new
      workload requires zero runtime-code changes").
- [ ] dev/test version runs through `bb check`; a stub-harness dogfood drive
      leaves a receipt.
- [ ] `./scripts/verify.sh` passes.

## Verification System

- Claim: refactor is a real diff-producing workload runnable from the CLI with
  no spine changes.
- Falsifier: it requires new `src/` code, edits behavior, or can't gate-green.
- Driver: copy the `build` template (task/agent/card), narrow the goal to
  structure; dogfood on a seeded messy file in the dev plane with a stub
  harness, then one live run on a real target.
- Grader: `bb check` validates the config; the live run's branch diff is a
  structural change with the gate green; ledger row shows cost/tokens.
- Evidence packet: branch diff + `bb runs show <id>` + gate output.
- Cadence: the live dispatch recipe in CLAUDE.md, run once per agent/model swap.

## Notes

**The named gap (groom 2026-06-17).** Today "refactor" exists only as the
read-only `simplification` storm lens (`plane/tasks/simplification/card.md` —
"never push, comment, merge, edit code"); the only diff-producing workload is
the generic `build`. This makes refactor a first-class dispatch workload — the
most direct answer to "the plane does more of our real work, refactors
included." Nearest template: `build` (shipped via 060); reuse its dispatch
path. The spine is workload-agnostic (CLAUDE.md: "a workload-specific branch in
dispatch/queue/substrate is wrong by definition"), so this is config, not
mechanism — if it needs `src/` changes, that's a signal the shape is wrong.
Not a child of 055 (copyable templates) or 062 (reflexes): it is a dispatch
authoring workload, a sibling of `build`.
