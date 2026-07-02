# 091 In-Flight Cap Enforcement Dogfood

Status: implementation proof complete on branch
`bb/build/091-inflight-cap-enforcement`.

## Goal

Make per-run spend caps real during execution: stream harness usage into the
ledger, kill or quarantine a run when it breaches `max_cost_per_run_usd`, and
escalate through the notification outbox.

## Dogfood Path

The production `build` task was parked by its daily run cap, so this slice did
not unpark it just to exercise BB. The live proof used a throwaway dev plane
with the same dispatch, local substrate, ledger, monitor, budget, and notify
paths:

- agent harness: `pi`
- policy: `side_effect_policy = "kill"`
- task budget: `max_cost_per_run_usd = 0.01`
- stub behavior: emit one assistant usage event with `cost.total = 0.02`, then
  sleep long enough that a non-killed run would be obvious
- notify transport: local stub appending webhook JSON to a log
- monitor interval: `BB_IN_FLIGHT_MONITOR_MS=50`

## Live Evidence

Run id: `490b20417ffb`

- final state: `failure`
- state reason: `in-flight cost cap kill: observed $0.0200 >
  max_cost_per_run_usd $0.01`
- duration: `216 ms`
- run cost: `$0.02`
- attempt outcome: `failure`
- attempt tokens: `tokens_in = 10`, `tokens_out = 5`, `turns = 1`
- attempt exit code: `-1` after process-group kill
- progress event: `cost observed $0.0200 elapsed=0s`
- notification outbox: one `run_in_flight_cap_killed` row, `delivered`,
  `attempts = 1`
- notification payload included `observed_cost_usd = 0.02`,
  `cap_usd = 0.01`, `policy = "kill"`, and task/agent/run identifiers
- status guard counts included `in_flight_cap_killed = 1`

Focused automated proof:

```bash
cargo test --test budgets in_flight_cost_cap_kills_running_harness_and_notifies -- --nocapture
cargo test --test budgets -- --nocapture
cargo test --test guardrails -- --nocapture
cargo test --test e2e_sprites -- --nocapture
```

All passed before the full repo gate.

The full gate's spine tripwire audited this as mechanism and reported
`src LOC: 9271` against the prior cap of `9000`. The added code lives in
dispatch, substrate, harness parsing, ledger stats, and status projection, and
does not encode workload judgment. Per the repo's bloat-tripwire doctrine, the
cap was raised narrowly to `9500` rather than golfing the runtime or inventing a
fake extraction.

## UX Notes

Good:

- The same notification outbox used by heartbeat/watchdog failures now carries
  spend-cap kills; no separate alert surface was invented.
- `bb status --json` exposes both configured enforcement mode and current
  in-flight spend/reservation, so an operator can see whether the belt is only
  declared or actively enforceable.
- The monitor updates the attempt row before termination, so the killed run
  still carries cost/tokens evidence instead of failing with an empty receipt.

Friction:

- Streaming enforcement depends on line-flushed usage events. The pi/omp
  wrapper had to stop using `grep -v` because grep buffered the stream and hid
  the overrun until process exit.
- Non-streaming harnesses remain final-cost only: they still park and notify
  after completion, but cannot be killed by spend before they report usage.

Residual:

- The remaining 091 oracle item is the broader loop-belt fixture: max wall clock
  and turn caps already exist where harnesses expose them, but max output bytes
  and max tool actions still need real enforcement if they stay in scope.
