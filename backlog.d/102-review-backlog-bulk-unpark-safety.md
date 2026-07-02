# Bug: bulk `task unpark` re-queues the entire historical blocked-budget backlog

Priority: P2 | Status: ready | Estimate: M

## Goal

Make re-enabling a budget-parked reflex task safe when its blocked-budget
backlog spans weeks, not just the run that just tripped the cap.

## Context (live incident, 2026-07-02)

While turning `review` (Cerberus-on-BB) on in production: `max_runs_per_day`
had been exhausted entirely by a wrapper bug (missing `--gh-token-env`, fixed
in #936) that made every dispatch fail in ~2-3s. Fixing the bug and raising
the cap left the task still parked (`bb task unpark` is a distinct, explicit
step from a budget-limit fix — by design, per `docs/operations/README.md`).

Running `bb task unpark review` re-queued **all 59** blocked-budget rows for
that task, not just the two runs the operator actually wanted — including
runs dating back to 2026-06-16 targeting PRs that were long since
merged/closed. One old run (against a merged PR from April) actually
dispatched and posted a real "Cerberus Review: PASS" comment to that stale,
closed PR before the operator could re-park the task
(`bb task park review --reason ...`). Recovery required scripting individual
`bb runs retire <id>` calls for the other 33 (see the run this incident
generated: idempotency keys `wh:review:...` dated 2026-06-12 through
2026-06-25).

This is mild in this incident (an advisory PASS comment on a closed PR is
harmless noise, not a merge/deploy/data mutation) but the blast radius is
task-shaped, not run-shaped, and a task with a lower `max_cost_per_run_usd`
ceiling or a mutating (non-advisory) authority tier would turn the same
footgun into real damage.

## Oracle

- [ ] `bb task unpark` reports (before acting, not just after) how many
      blocked-budget runs it is about to re-queue and their age range, or
      requires an explicit `--yes`/confirmation once the count exceeds a
      small default.
- [ ] A scoped release path exists for "re-queue only runs newer than X" or
      "re-queue only these N run ids" without hand-writing a retire-loop
      script, e.g. `bb task unpark --since <timestamp>` or `--max-age`.
- [ ] `docs/operations/README.md` gains a runbook note: before unparking a
      task that has been parked for a while, check `bb runs list --task
      <task> --state blocked_budget --json` and consider `bb runs retire`
      for anything targeting closed/merged/stale externals first.

## Secondary finding (unresolved, needs investigation)

`bb runs release <run_id>` failed for one of the two target runs in this
incident with `run <id> is held by a budget limit on task 'review', not a
park; release cannot clear it` — even though `bb task list --json` showed
`parked: null`, `runs_today` well under `max_runs_per_day`, and
`cost_today_usd` well under `max_cost_per_day_usd` at the same moment (and
manually recomputing the `runs_today` SQL predicate over the full runs list
agreed with the reported value). Retried three times over ~2 minutes with the
same failure. Worked around by retiring that run and using the `manual`
trigger (`bb run review --payload ...`) instead, which dispatched
successfully. Root cause not found — worth a fresh look at
`budget::budget_limits` and whether `runs release`'s pre-check can diverge
from `task list`'s live view (possibly a `Plane::load`/`Ledger::open`
staleness or transaction-visibility gap, not reproduced in isolation).

## Evidence

- Backlog 093 (Cerberus-on-BB) live wiring session, 2026-07-02: bb#936,
  bb#937, real advisory reviews posted to misty-step/bitterblossom#842,
  #936, and misty-step/powder#31.
