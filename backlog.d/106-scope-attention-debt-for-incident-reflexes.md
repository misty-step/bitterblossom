# Scope attention-debt admission so incident reflexes are not blocked by unrelated parked work

Priority: P0 | Status: ready | Estimate: M

## Goal

`incident-triage` webhook deliveries should not be refused solely because an
unrelated task family has parked budget debt. Incident responders need their own
admission policy: bounded by their own task caps and DLQ state, but not held
behind stale review-volume debt.

## Evidence

Live drill on 2026-07-02:

- Canary incident: `INC-ay76lctwao3z` on service `powder`
- Webhook delivery: `DLV-zo93xxqrjhsd`
- BB task: `incident-triage`
- Delivery result: discarded after 4 attempts with `reason=http_429`
- BB status reason: `attention_debt_brake` because `open_dlq=1 parked_tasks=1`
- Parked task causing the global brake: `review`, `32 runs today >= max_runs_per_day 20`
- Open DLQ causing the global brake: stale `canary-triage` run
  `2e3013116b4c`, branch `factory/bitterblossom-lane-20260701` no longer
  exists

The delivery matched the new route and secret, but no `incident-triage` run was
created.

## Oracle

- [ ] Add a task-level or priority-aware admission policy that lets
      `incident-triage` accept a webhook when its own task is not parked and its
      own daily cap is available, even if lower-priority/unrelated tasks are
      parked.
- [ ] Preserve the global brake for unsafe broad automation; do not remove debt
      visibility from `bb status --json`.
- [ ] Add an ingress test where `review` is parked and `incident-triage` still
      ingests a matching signed webhook.
- [ ] Add a negative test where `incident-triage` itself is parked or over cap
      and the webhook is still refused.
- [ ] `./scripts/verify.sh` passes.

## Non-goals

Do not silently unpark `review`, replay old DLQ rows, or erase debt to make
incident admission look green. The fix is scoped admission semantics.
