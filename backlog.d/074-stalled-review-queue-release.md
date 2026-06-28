# Release the stalled review queue from blocked-budget limbo

Priority: P1 | Status: ready | Estimate: M

## Goal

Stop new webhook-triggered review runs from entering `blocked_budget` state with `agent_name=null` (no agent assignment) and reason "task parked: verification paused after stale queue release". Twenty-five runs have accumulated in this state over 10 days, representing every webhook review trigger during that period. Recovered operators should either proceed to an assigned reviewer or fail with a clear diagnostic — not silently pile up as budget-blocked orphans.

## Oracle

- [ ] A webhook-triggered review run created after the fix is assigned to `cerberus-reviewer@v1` (or another concrete agent) and progresses past the `blocked_budget` gate.
- [ ] No new `blocked_budget` runs with `agent_name=null` and the "stale queue release" reason appear for at least 7 consecutive days after deployment.
- [ ] The 25 existing orphaned runs are either released, failed with an informative `state_reason`, or explicitly garbage-collected — not left in permanent limbo.
- [ ] The budget gate that produced `blocked_budget` for these runs is documented in a comment or dash so an operator can distinguish a genuine budget ceiling from a queue-assignment failure.
- [ ] (Secondary) Cerberus-reviewer tooling failures (`changeType` parsing, missing `opencode`, expired GH token) produce better error messages that distinguish infra rot from code bugs, or the relevant env/binary dependencies are verified in a preflight check before the review lane starts.

## Notes

### The blocked-budget cascade

All 25 `blocked_budget` runs share identical characteristics:

- `task`: `review`
- `state`: `blocked_budget`
- `state_reason`: `"task parked: verification paused after stale queue release"`
- `agent_name`: `null`
- `trigger_kind`: `webhook` (all webhook-triggered PR reviews)
- `agent_version`: `null`

The runs span **2026-06-15 through 2026-06-25** and were all last `updated_at` around **2026-06-25T17:28:XX** — suggesting a bulk release attempt that failed to make progress. Every review webhook fired to the plane during this 10-day window entered this dead-end state. No review run from a webhook trigger succeeded or even reached an agent.

Breakdown by creation date:
- 2026-06-15: 1 run
- 2026-06-16: 16 runs
- 2026-06-17: 4 runs
- 2026-06-20: 2 runs
- 2026-06-21: 1 run
- 2026-06-25: 1 run

This is a **100 % blocking rate** for webhook reviews over 10 days with no natural recovery.

### Cerberus-reviewer tooling regressions

Six additional review failures in the same 14-day window show `cerberus-reviewer@v1` suffering from at least three distinct infra/tooling problems:

| Run ID | Failure | Root cause |
|--------|---------|------------|
| `7d20a25b5f36`, `7af49577152f` | `gh` CLI HTTP 401 | Expired/misconfigured GitHub token in the cerberus environment |
| `7f8fceefdcc1`, `17591327a7a9` | Missing `changeType` field in `gh pr view --json` | GitHub API schema drift — the `gh` output no longer includes the expected field |
| `9a9c12096eb9` | `opencode` binary not found | Missing tool dependency in the cerberus harness PATH |
| `2b5bb2c768cc` | Missing review artifact file | Build step did not produce the expected review artifact |

These are all *mechanical* — each can be detected before a PR author waits for review and gets a cryptic `harness exit 1` error.

### Cost outlier

Run `516385b5f22c` (review-coordinator, 2026-06-15) cost **$0.97**, exceeding the agent's `max_cost_per_run_usd` of $0.75 by 29 %. The median cost of successful review-coordinator runs is $0.12. This single run cost more than the 10 cheapest reviews combined. Its duration (9.3 min) was also above average. If this is caused by LLM retry loops or oversized context, a per-agent cost cap should have terminated it earlier.

### Data gaps

- The `/api/submissions` endpoint returned an empty array — no verdict or gate-report data was available. This limited the analysis to run-level metadata only. A richer submissions/verdict endpoint would enable stronger findings about reviewer quality and blocking-finding overrule rates.
- The `/api/dlq` endpoint was empty, which is healthy.

## Evidence

### Blocked-budget runs (25 runs, no agent assignment)

```
bd8dda640578  e2610fbbe1fd  4075fb12e4f1  6d682c4f1ab2  5776c30ccbed
1bb01669398f  fb379e102ece  fb16690dd35b  4f107f3d5902  834415072521
f82942338f6b  0d8ddf688bbd  d7292f4acb7c  5dcfe85288d2  b13457a78779
93737fc73f87  ef7a883ec9d8  70a01b5c5032  2f4cd84dff96  42d8838bba20
5f267b602007  1f662bb2dc94  c42f6989bcce  b934d5698686  287927ccc03b
```

All have `state=blocked_budget`, `agent_name=null`, `state_reason="task parked: verification paused after stale queue release"`, `trigger_kind=webhook`, `task=review`.

### Cerberus-reviewer toolchain failures (6 runs)

- `7d20a25b5f36`, `7af49577152f` — `gh` auth failure (HTTP 401) against `api.github.com/graphql`
- `7f8fceefdcc1`, `17591327a7a9` — `gh pr view --json` output missing `changeType` field
- `9a9c12096eb9` — `opencode` binary not in trusted PATH
- `2b5bb2c768cc` — review artifact file not found at expected path

All use `agent_name=cerberus-reviewer@v1`, `harness=command`, `task=review`.

### Cost outlier

- `516385b5f22c` — review-coordinator, $0.9708, 557775ms, created 2026-06-15T21:24:58