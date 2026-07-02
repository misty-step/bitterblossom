# Epic: per-agent governance and real circuit breakers

Priority: P0 | Status: ready | Estimate: XL

## Goal

Give every BB agent definition its own authority and spend boundary. The plane
must be able to mint, track, and enforce scoped model keys and stop runaway agent
loops before they consume budget or authority.

## Oracle

- [x] Each API-auth agent can use a discrete OpenRouter API key or equivalent
      BYOK credential scoped to that agent definition.
- [x] Key provisioning is automated through the OpenRouter provisioning API or a
      documented manual fallback with the same ledger-visible fields.
- [ ] Agent budget lines map to provider-side spend caps where supported, and BB
      status shows configured cap, reserved spend, spent today, and enforcement
      mode.
- [ ] Iteration/turn caps, max wall-clock, max output bytes, and max tool actions
      are enforceable per agent/task before a run starts.
- [ ] In-flight overrun handling can kill or quarantine a run according to a
      configured side-effect policy, then records a recovery action.
- [x] Per-agent policy is visible in `bb check --json`, `bb task list --json`,
      and `/api/tasks`.
- [ ] `./scripts/verify.sh` passes.

## Children

- [x] Agent policy schema: authority, provider key name, budget cap, iteration
      caps, timeout, and side-effect policy.
- [x] OpenRouter key provisioning or audited manual import path.
- [ ] Provider cap sync and drift check.
- [ ] In-flight kill/quarantine mechanism for overrun policies.
- [ ] Status/API/readiness projection of governance state.
- [ ] Fixtures proving an infinite loop is stopped by code, not by operator luck.

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
