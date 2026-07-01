# Freshness Contracts

Bitterblossom treats unattended silence as operator work. The status surface
emits the live machine-readable copy of this table as
`bb status --json | jq '.freshness_contracts'`.

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

The executing boundary is intentionally conservative: an agent at or after
`attempt.executing` may have external side effects. The plane may probe and
escalate, but it does not blindly replay or kill that work as a remediation.
