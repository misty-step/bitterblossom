# Epic: per-agent governance and real circuit breakers

Priority: P0 | Status: done | Estimate: XL

## Goal

Give every BB agent definition its own authority and spend boundary. The plane
must be able to mint, track, and enforce scoped model keys and stop runaway agent
loops before they consume budget or authority.

## Oracle

- [x] Each API-auth agent can use a discrete OpenRouter API key or equivalent
      BYOK credential scoped to that agent definition.
- [x] Key provisioning is automated through the OpenRouter provisioning API or a
      documented manual fallback with the same ledger-visible fields.
- [x] Agent budget lines map to provider-side spend caps where supported, and BB
      status shows configured cap, reserved spend, spent today, and enforcement
      mode.
- [x] Iteration/turn caps, max wall-clock, max output bytes, and max tool actions
      are enforceable per agent/task before a run starts.
- [x] In-flight overrun handling can kill or quarantine a run according to a
      configured side-effect policy, then records a recovery action.
- [x] Per-agent policy is visible in `bb check --json`, `bb task list --json`,
      and `/api/tasks`.
- [x] `./scripts/verify.sh` passes.

## Children

- [x] Agent policy schema: authority, provider key name, budget cap, iteration
      caps, timeout, and side-effect policy.
- [x] OpenRouter key provisioning or audited manual import path.
- [x] Provider cap sync and drift check.
- [x] In-flight kill/quarantine mechanism for overrun policies.
- [x] Status/API/readiness projection of governance state.
- [x] Fixtures proving an infinite loop is stopped by code, not by operator luck.

## Notes

This epic turns the word "circuit breaker" into code. It is distinct from global
daily budget admission: provider-side caps and per-agent loop belts prevent one
bad agent from exhausting the whole plane.

2026-07-02 slice: `bb keys mint|rotate|revoke|list` provisions OpenRouter child
keys from agent policy caps using `OPENROUTER_MANAGEMENT_KEY`, stores child
keys under plane `.bb/`, injects them per run, and refuses shared-key fallback
for policy-bound OpenRouter agents. Live proof: `bb-builder-rust` minted hash
`2693df917b62ba4de9c1bf339cb881ae97f0f98f3be3d7533b697ec237d089ed`, remote
list showed `limit = 25.0`, `limit_remaining = 25.0`, `disabled = false`.

2026-07-02 slice: dispatch now streams harness usage into attempt stats while a
run is executing. When observed cost exceeds `max_cost_per_run_usd`, `kill`
terminates the harness and emits `run_in_flight_cap_killed`; `quarantine` moves
the run to recovery; `log` records the breach and lets it continue. Focused
proof fixture: `in_flight_cost_cap_kills_running_harness_and_notifies`.

2026-07-02 slice: agent policy and task budgets now compose into one effective
runtime budget before dispatch: wall-clock uses the strictest timeout, Claude
gets the strictest native `--max-turns`, JSONL harnesses stream turn/tool/cost
progress into the monitor, and output bytes are counted from stdout/stderr.
Unsupported turn/iteration/tool caps fail before command construction instead
of becoming pretend guardrails on generic `command` agents. Live proof fixtures:
`turn_policy_cap_kills_running_harness_and_notifies`,
`tool_action_policy_cap_kills_running_harness_and_notifies`, and
`output_bytes_policy_cap_kills_running_harness_and_notifies`; full proof:
`./scripts/verify.sh` green with `src LOC: 9913` under the raised 9950 mechanism
tripwire.

2026-07-02 slice: `bb keys sync <agent>|--all --check --json` refreshes
plane-side non-secret OpenRouter key metadata from the management list endpoint,
compares policy caps to the remote `limit`, and exits non-zero with a structured
drift report when caps, missing keys, disabled state, or local key material do
not match. `bb check --json`, `bb task list --json`, `/api/tasks`, and
`bb status --json` now expose last-synced provider key status without requiring
the management key on ordinary read calls. Focused proof fixture:
`key_sync_detects_remote_cap_drift_and_updates_local_metadata_without_printing_secret`.
Full proof: `./scripts/verify.sh` green with `src LOC: 10318` under the raised
10500 mechanism tripwire. Epic complete.
