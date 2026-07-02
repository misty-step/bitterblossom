# Stress and Recovery Evidence for Build-vs-Borrow

Date: 2026-07-02
Backlog: 058 child 1

## Purpose

This memo captures the live stress and recovery evidence produced by the
050/051 hardening work so the durable-workflow and substrate re-check starts
from repo evidence instead of premise arguments. It does not compare vendors
yet and does not add a runtime dependency.

## Evidence Sources

- `backlog.d/_done/050-event-plane-hardening-before-growth.md`
- `backlog.d/_done/051-recovery-and-substrate-probes.md`
- `docs/spine.md`
- `docs/freshness-contracts.md`

## Stress Evidence From 050

| Surface | Evidence | Build-vs-borrow implication |
|---|---|---|
| Ingress durability and budget pressure | `scripts/control-loop-drill.sh` fired five signed webhook deliveries against a task with `max_runs_per_day = 1`: one run reached `success`, four became `blocked_budget`, the task parked with a concrete reason, and four `budget_blocked` notifications were recorded. | Any borrowed queue/runtime must preserve ledger-before-ack semantics, per-task budget admission, parked state, and operator-visible blocked rows. A generic retry queue is not enough if over-budget work disappears or is only visible in logs. |
| Read-surface auth under loopback/public modes | The same control-loop drill verified open loopback reads, then `BB_API_TOKEN` mode where missing, bad bearer, and query-token reads return `401`, while bearer `/api/status`, `/api/tasks`, `/api/runs`, `/api/dlq`, `/api/submissions`, and `/` return `200`. | External dashboards or workflow consoles do not replace the plane's operator API contract. Any borrowed visibility layer must not reintroduce token-in-URL or split the CLI/API truth source. |
| Worker panic cleanup | `tests/serve.rs` seeds two pending runs for the same task, forces the first worker to panic after dispatch, and proves the next pending run drains to `success`. | The plane needs panic/failure cleanup around the host/task in-flight guard. A workflow runtime can help with worker supervision, but it must still compose with host mutual exclusion and per-task FIFO. |
| Notification storm accounting | `tests/budgets.rs` runs a burst through a slow fake notification binary and proves all notification children are accounted before `notify()` returns. | Borrowed notification/outbox primitives are attractive only if they keep fan-out bounded and auditable. Fire-and-forget side effects are a regression. |
| CLI/docs parity | `tests/cli_contract_docs.rs` executes live `bb` help and checks current docs/skills for matching examples. | The operator contract is a product surface. Vendor migration has to preserve stable `--json` CLI affordances, not only service internals. |

## Recovery Evidence From 051

| Surface | Evidence | Build-vs-borrow implication |
|---|---|---|
| Probe state machine | `docs/spine.md` defines local and sprite probe states as `alive`, `dead`, and `unknown`; `recover --json` exposes `probe_state`, `probe_reason`, `lease_disposition`, and `operator_action`. | The hard part is not retrying a job. The hard part is classifying side-effecting coding-harness work after process or host uncertainty. A borrowed runtime must either expose enough state to support this probe contract or stay below it as an implementation detail. |
| Unknown evidence retention | Recovery tests cover missing local pid markers, malformed pidfiles, and sprite probe command failure as `unknown`, retaining the lease instead of falsely freeing the host. | False-dead recovery is worse than sticky uncertainty because it can overlap side-effecting agents on the same workspace/host. Any substrate adapter must score lease retention and unknown-state visibility directly. |
| Stale awaiting-recovery visibility | `bb status --json` changes the safe action from `resolve_after_side_effect_inspection` to `escalate_stale_recovery` after one hour and includes `age_seconds` plus `stale_after_seconds`. | Durable workflow visibility is useful, but the accepted behavior is operator-action classification, not automatic replay of side-effecting work. |
| Operator resolution boundary | `docs/spine.md` states failures before `executing` retry mechanically, while failures at or after `executing` become `failure` or `awaiting_recovery`; replay is explicit operator action. | Temporal-style replay semantics may help pre-execute orchestration, but they must not hide side effects behind automatic re-run behavior once a coding harness has started. |

## Current Baseline To Score Against

The current custom spine has proven these primitives locally:

- durable run row before trigger acknowledgement;
- signed webhook dedupe and per-task budget blocking;
- per-task FIFO layered over host mutual exclusion;
- bounded in-flight cleanup after panic;
- bounded notification execution;
- explicit pre-execute vs executing recovery semantics;
- structured recovery probes for local and sprite substrates;
- stale recovery visibility through `bb status --json`;
- stable CLI/API read surfaces and doc parity checks.

The comparison slices should treat those as the minimum bar. A borrowed
workflow runtime can win if it reduces queue, retry, cron, observability, or
state-resume code without weakening the coding-harness substrate contract. A
substrate can win if it prepares a real repo workspace, streams stdout/stderr,
passes secrets on stdin, captures artifacts, supports timeout/kill, and
survives uncertainty at least as explicitly as the current local/sprite probes.

## Open Evidence Gaps For Later 058 Slices

- No current vendor comparison has been made in this memo.
- No adapter probes were run in this slice.
- No overnight sprite dispatch was used; this slice relies on the already
  landed 050/051 local and documented live-loop evidence.
- The next slice should turn this evidence into a primitive-by-primitive
  scoring rubric before touching any candidate runtime.
