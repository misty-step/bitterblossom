# Expose meaningful progress and stale-action signals for long-running attempts

Priority: P1 · Status: ready · Estimate: M

## Goal

Make long-running BB attempts operator- and agent-readable enough that a supervisor can decide whether to wait, recover, escalate, or dispatch follow-up work without guessing from a stale `running` row.

## Problem / Dogfood Evidence

During the 079 artifact CLI dogfood run, `bb runs show 825ba972a832 --json` remained:

```text
state=running
attempt.phase=executing
artifact_dir=null
updated_at=2026-06-30T01:59:22Z
events: state:running, boot_probe alive
```

Hours later this was still technically alive, but not enough for an overnight supervisor to know whether the agent was making meaningful progress, waiting on a model, hung in remote execution, or safe to escalate. A later `bb recover --json` classified the run as dead/`awaiting_recovery`; side-effect inspection found the pushed branch, but the original run still had no artifact directory/REPORT for closeout. That split-brain outcome is exactly the missing product signal: pushed side effects can exist while BB's run row lacks meaningful progress/artifact evidence.

## Oracle

- [ ] Running attempts expose last heartbeat/progress time separate from run creation/update time.
- [ ] `bb runs show --json` and `bb status --json` include a stale-progress classification with `age_seconds`, threshold, and safe next action.
- [ ] The dispatcher or harness captures lightweight progress markers when available: remote process alive, bytes/log lines changed, artifact dir created, model turn count/tokens when reported, or explicit harness heartbeat.
- [ ] Lack of meaningful progress does not automatically kill executing work with external side effects; it produces an operator-visible `awaiting_recovery`/`stale_executing` recommendation according to policy.
- [ ] Tests cover fresh running, alive-but-no-progress, dead pre-attempt, dead executing, and unknown probe outcomes.
- [ ] `./scripts/verify.sh` passes.

## Verification System

- Claim: a Hermes supervisor or human operator can tell the difference between “still working,” “alive but no meaningful progress,” “dead before execute,” and “needs operator recovery.”
- Falsifier: a run can sit `running` for hours with no age/progress classification; the only way to decide is private process inspection; or a stale executing run is retried automatically despite possible side effects.
- Driver: local-plane fixture task with heartbeat, a fixture task that sleeps without progress, and a simulated inherited executing run.
- Grader: status JSON contains machine-readable safe next actions; no automatic rerun occurs for executing side-effect-capable work; operator commands are explicit.
- Evidence packet: status JSON snapshots for each fixture state and one live long-running run transcript.
- Cadence: before enabling Hermes supervisor Level 1+ or autonomous dispatch loops.

## Promotion Metrics

This must land before the dogfood supervisor may move beyond read-only monitoring:

- 5 monitor reports classify running/stale states using BB JSON, not `ps` spelunking;
- stale threshold is configurable per task family or documented globally;
- every stale recommendation includes `safe_next_command` or an explicit “ask operator” blocker.

## Notes

Related to 051 and 083, but this ticket is scoped to the product surface exposed to operators/agents for active long-running attempts. It is the missing signal that made the overnight dogfood loop unsafe to automate.
