# Epic: ad hoc agents for arbitrary repo work

Priority: P1 | Status: ready | Estimate: XL

## Goal

Make "spin up an agent to do arbitrary bounded work on a repo, then shut it
down" a first-class BB path so heavy execution can move off the laptop without
hand-built shell wrappers.

## Oracle

- [ ] CLI and MCP surfaces can create a bounded ad hoc run from repo, base ref,
      lane card or prompt, agent binding, budget, authority, and artifact
      requirements.
- [ ] The plane prepares an isolated workspace, runs the agent, captures artifacts,
      and tears down or parks resources according to policy.
- [ ] The run can optionally open a branch/PR but cannot merge unless a separate
      scorecard and operator policy allow it.
- [ ] Duplicate active work is refused for the same repo/task family unless
      forced with a recorded reason.
- [ ] Receipts include run id, workspace/substrate, branch/PR if any, cost,
      artifact handles, and safe next action.
- [ ] A local fixture and one Sprite-backed dogfood run prove the path.
- [ ] `./scripts/verify.sh` passes.

## Children

- [ ] Ad hoc run spec and validation.
- [ ] CLI command or recipe that avoids unsafe argv secrets and manual JSON
      quoting.
- [ ] Mutating MCP tool gated by explicit authority and idempotency.
- [ ] Workspace lifecycle and cleanup policy.
- [ ] Duplicate active-run refusal.
- [ ] Dogfood on one non-Bitterblossom repo backlog item.

## Notes

This is the laptop-RAM relief path named in the Factory report. It builds on the
builder dogfood loop but makes arbitrary bounded commissions product-native.
