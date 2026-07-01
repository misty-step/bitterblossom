# Freshness Contracts

Bitterblossom treats unattended silence as operator work. The status surface
emits the run/attempt machine-readable contracts as
`bb status --json | jq '.freshness_contracts'`; gate-arm policy is live under
`bb status --json | jq '.guards.gate'`.

| Subject | Threshold | Owner | Safe next action | Notify severity |
|---|---:|---|---|---|
| `run.pending` | none | dispatcher | `bb runs list --state pending --json` | none |
| `run.running` | 1800s | operator | `bb recover --json` before resolving or killing | warning |
| `run.awaiting_recovery` | 3600s | operator | `bb runs show <id> --json`, inspect side effects, then `bb runs resolve` | critical |
| `run.blocked_budget` | none | operator | `bb runs release <id>` or `bb runs retire <id> --reason TEXT` | warning |
| `attempt.acquired` | 1800s | dispatcher | `bb runs show <id> --json`; `bb recover --json` if stale | warning |
| `attempt.prepared` | 1800s | dispatcher | `bb runs show <id> --json`; `bb recover --json` if stale | warning |
| `attempt.executing` | 1800s | operator | `bb recover --json`; never auto-replay without side-effect inspection | critical |
| `attempt.collecting` | 1800s | dispatcher | `bb runs show <id> --json`; `bb recover --json` if stale | warning |
| `attempt.finalizing` | 1800s | dispatcher | `bb runs show <id> --json`; `bb recover --json` if stale | warning |
| `attempt.released` | 1800s | dispatcher | `bb runs show <id> --json`; `bb recover --json` if stale | warning |
| `notification.pending_or_failed` | none | operator | `bb notify retry --json` or `bb notify ack <id> --reason TEXT --json` | warning |
| `submission.gate_arm` | `[gate].arm_timeout_seconds` | gate | `bb gate --submission <id> --json`, then inspect the member `safe_next_*` fields | warning/critical |

The executing boundary is intentionally conservative: an agent at or after
`attempt.executing` may have external side effects. The plane may probe and
escalate, but it does not blindly replay or kill that work as a remediation.

`bb serve` runs a watchdog alongside dispatch, cron, and notification retry.
When a running attempt classifies as `stale_executing`, the watchdog emits a
durable `run_stale_executing` notification through the outbox and records a
`watchdog:stale_notified` run event as the dedupe marker. Repeated scans do not
create duplicate notifications unless the run records new progress and then goes
stale again.

Submission gate arms use the plane's `[gate].arm_timeout_seconds` policy
(default 3600s). A member that is not started or whose canonical storm run stops
changing past that threshold cannot keep `bb gate` pending forever: the gate
either escalates through `submission_escalated` when quorum cannot be met, or
settles with a separate `submission_arm_timed_out` notification when an explicit
lower quorum allows the decision to proceed.
