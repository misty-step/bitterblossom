# Bitterblossom Dogfood UX Review Card

Use during dogfood reflection, after the `bb` run evidence is available.

## Questions

- Does it work? Did `bb` produce a branch/PR, ledger rows, artifacts, costs,
  and a gate result without hidden manual repair?
- Does it produce useful results? Was the PR reviewable, narrow, verified, and
  traceable back to the backlog item?
- Does it feel good? Did the CLI communicate progress, next actions, and
  blocked states at the moment they mattered?
- Is it too complicated or awkward? Count steps that exist only because the
  plane lacks a better affordance.
- Were there errors? Separate product bugs from operator setup mistakes, then
  ask whether the product should have preflighted or explained the mistake.
- Was communication unclear? Name the command/output that forced guessing.
- Were there more steps than necessary? Point to the command that could absorb
  them without moving workload judgment into Rust.

## Backlog-worthy

Emit or update backlog only when all are true:

- It affects the documented vision: tasks/agents/triggers as files, terminal
  dispatch, durable ledger, costs, gates, or operator truth surfaces.
- It is likely to repeat for another operator or agent.
- It has a concrete oracle: command, JSON field, artifact, route, or evidence
  path that can prove the improvement.
- The fix belongs in Bitterblossom, not in one task card's private judgment.

## Keep

Record positive friction reducers as `Lean in` when they should remain true:
stable `--json`, early run ids, safe next commands, artifact paths, cheap
manual dispatch, or reviewable PR output.
