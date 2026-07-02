# Required Primitives for the Build-vs-Borrow Re-check

Date: 2026-07-02
Backlog: 058 child 2

## Purpose

This memo defines the primitives a workflow runtime or substrate candidate must
cover before the 058 decision memo compares specific products. It is a scoring
baseline, not a vendor verdict.

## Primitive Comparison Baseline

| Primitive | Bitterblossom requirement | Current owner | Borrowing posture |
|---|---|---|---|
| Queue and admission | A run row exists before trigger acknowledgement; webhook dedupe and cron dedupe are durable; budget-blocked work is recorded as `blocked_budget`, not dropped; per-task FIFO is preserved. | `ingress`, `ledger`, `serve`, task budget policy. | Borrow only if the external queue can be made subordinate to the ledger contract. A queue that accepts work before `bb` records it, hides over-budget events, or loses idempotency evidence fails the baseline. |
| Mechanical retries | Pre-`executing` failures retry mechanically with bounded attempts and dead-letter history; at/after-`executing` failures do not auto-replay because harnesses have side effects. | `dispatch`, `ledger`, `recovery`, `dlq`. | Borrow retry plumbing for pre-execute phases if it preserves attempt history and parent/child replay lineage. Do not borrow generic replay semantics for executing harness work. |
| Leases and concurrency | Host mutual exclusion is durable and keyed by substrate resource; per-task FIFO sits above host leases; in-memory in-flight guards are panic-cleaned. | `ledger` host leases plus `serve` in-flight guards. | Mostly owned by `bb`. A runtime scheduler may help dispatch workers, but the lease authority must stay where substrate identity and side-effect recovery are visible. |
| Recovery classification | Inherited `running` rows are probed, never blindly orphaned; `alive`, `dead`, and `unknown` are explicit; `unknown` retains the lease; operator action is machine-readable. | `recovery`, `substrate/local`, `substrate/sprites`, status projection. | Owned by `bb` unless a substrate runtime exposes equal or better process/workspace proof. A workflow engine's task retry state is not a substitute for host/process evidence. |
| Visibility | Operators and agents read stable `--json` CLI/API shapes for status, runs, attempts, DLQs, artifacts, progress, freshness, and submissions; notifications are durable outbox rows and retryable. | `cli`, `serve`, `status`, `notify`, docs/skills contract tests. | Borrow dashboards or telemetry sinks only as secondary views. The CLI/API contract remains canonical so agents do not depend on a vendor console. |
| Cost tracking and caps | Agent budget lines, per-run caps, in-flight reserved spend, provider usage, blocked-budget rows, and cap-breach notification are enforced by the plane. | `budget`, `policy`, `dispatch`, `ledger`, provider key management. | Provider APIs can supply usage data, but admission, cap decisions, and kill/escalate behavior stay in `bb`. A workflow runtime cannot be the budget authority unless it can meter harness spend mid-run. |
| Sandbox dispatch | The workload prepares a real repo workspace, streams stdout/stderr, sends prompts/secrets on stdin, scrubs inherited env, records artifacts, supports timeout/kill, and preserves enough probe state for recovery. | `substrate` adapters and harness command builders. | Candidate substrates can win here. The adapter boundary is the correct replacement point, but the candidate must pass the same coding-harness contract instead of only running arbitrary functions. |
| File-first workload config | Tasks, agents, triggers, policies, budgets, and repo bindings are files; workload judgment stays out of the runtime spine; product/instance data is runtime config, not image content. | `spec`, `policy`, examples, `scripts/verify.sh` product/instance guard. | Owned by `bb`. Borrowed systems may store derived state, but the source of truth must remain repo/runtime config that agents can inspect and version. |

## Candidate Scoring Questions

Each workflow runtime or substrate candidate must answer these questions in
the decision memo:

1. Does the candidate receive work only after `bb` durably records the run, or
   can it participate in that transaction without hiding failures?
2. Can it express bounded pre-execute retries while refusing automatic replay
   once a coding harness may have touched a repo, PR, issue, or external
   system?
3. Can it preserve host/workspace mutual exclusion across process crashes,
   deploys, and operator restarts?
4. What exact evidence does it provide when a run is uncertain: process alive,
   process dead, lost marker, failed probe, missing workspace, or hung stdout?
5. Can its visibility be projected into `bb status --json`, `bb runs show
   --json`, `bb dlq list --json`, and outbox notifications without making a
   vendor console the source of truth?
6. Can it meter or bound spend while a run is executing, and can `bb` kill the
   run on cap breach?
7. Does it support stdin-carried prompts/secrets, artifact extraction, and
   stdout/stderr streaming without leaking secrets through argv, logs, or image
   layers?
8. Does adopting it keep workload logic in task cards and agent bindings, or
   would it move judgment into runtime code?

## Borrowing Boundaries

Primitives that are plausible to borrow narrowly:

- cron scheduling mechanics;
- queue wakeup and worker supervision;
- pre-execute retry timers;
- secondary observability export;
- sandbox provisioning, if the substrate contract is stronger than the current
  Fly/Sprites baseline.

Primitives that must remain plane-owned unless a candidate proves an identical
operator contract:

- ledger-before-ack acceptance;
- budget admission and cap-breach enforcement;
- host/workspace lease authority;
- executing-phase recovery policy;
- stable agent-facing CLI/API JSON;
- file-first task/agent/trigger source of truth.

## Next Slice Input

The next 058 slice should convert these primitives into a repeatable
coding-harness workload and scoring rubric. Candidate-specific research and
adapter probes should wait until that rubric exists.
