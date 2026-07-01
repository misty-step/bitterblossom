# 091 Per-Agent Governance — Schema & Read-Surface Dogfood

Status: first slice complete. Branch: `bb/build/091-agent-policy-schema`.

## Context

- Goal (backlog 091): give every BB agent definition its own authority and spend
  boundary, with real circuit breakers that stop runaway loops before they burn
  budget or authority.
- This slice only: the **schema** and **read surface**. An optional per-agent
  `[policy]` table is parsed from agent TOML, validated at config load, and
  projected read-only through `bb check --json`, `bb task list --json`, and
  `/api/tasks`.
- Explicitly out of scope (deferred to later 091 slices): OpenRouter key
  provisioning/minting, provider cap sync, in-flight kill/quarantine, and moving
  `plane/` config. No provisioning APIs are called.

## Schema

The optional `[policy]` table on an agent TOML:

```toml
[policy]
authority = "edit"                  # read | edit | merge
provider_key_name = "openrouter-builder"
provider_spend_cap_usd = 25.0
model_allowlist = ["z-ai/glm-5.2"]
trigger_bindings = ["manual"]       # manual | cron | webhook
iteration_cap = 1
turn_cap = 120
tool_action_cap = 200
output_bytes_cap = 262144
wall_clock_minutes = 45
side_effect_policy = "log"          # log | quarantine | kill
```

All fields optional; the table itself is optional. Absent → all-`None`/empty
defaults, fully backward compatible.

## Validation (at `bb check` / config load, not dispatch)

- `authority` must be `read`/`edit`/`merge`.
- `side_effect_policy` must be `log`/`quarantine`/`kill`.
- `provider_spend_cap_usd` must be non-negative.
- every cap (`iteration_cap`, `turn_cap`, `tool_action_cap`,
  `output_bytes_cap`, `wall_clock_minutes`) must be greater than zero.
- `trigger_bindings` entries must be `manual`/`cron`/`webhook` and unique.
- **model allowlist mismatch**: if `model_allowlist` is non-empty and the agent
  has a model, the model must be a member.

## Read Surface

`tasks_view` (shared by `bb check --json`'s `task_details`, `bb task list
--json`, and `/api/tasks`) now carries a `policy` object per task. `check_view`
adds an `agent_policy` map keyed by agent name. Same shape everywhere, so MCP
`bb_check` and the API never build their own.

## Dogfood Evidence

- Added a real `[policy]` to `plane/agents/bb-builder-rust.toml` — the agent
  that dispatches this very build. It may edit/push but never merge, its model
  is pinned to `z-ai/glm-5.2`, and its loop/spend surface is bounded.
- `bb --config plane check --json` projects the policy: builder
  `authority=edit`, `provider_spend_cap_usd=25.0`, `model_allowlist=["z-ai/glm-5.2"]`,
  and the `build` task's `task_details[*].policy.authority=edit`.
- `bb --config plane task list --json`: 1 of 32 tasks carries a policy table;
  the other 31 project `authority: null` (backward compatible).
- Tests (`tests/policy.rs`): valid load + projection; absent defaults; model
  allowlist mismatch; unknown authority/side_effect; zero cap; negative spend;
  unknown + duplicate trigger bindings.

## Verification

- `cargo fmt --check`, `cargo clippy --all-targets -- -D warnings`, and the
  policy test suite pass.
- Full gate: `./scripts/verify.sh` (run on this branch; see REPORT.json).

## UX Notes

### Good

- The `[policy]` table lives next to the agent it bounds — governance is
  co-located with the agent definition, not a separate registry. Reads naturally.
- Reusing `tasks_view` for all three read surfaces meant one projection point;
  no per-surface shape drift.

### Friction

- `authority` is a free string validated against an enum. A typo like `"edt"`
  fails at `bb check` with a clear message, but there is no editor/autocomplete
  aid. Acceptable for a first slice; a typed enum in TOML tooling is a later
  polish.
- The model-allowlist mismatch check is string-equality on the model id. If
  provider aliasing ever lands (`glm-5.2` vs `z-ai/glm-5.2`), this check needs a
  normalizer, not a raw compare.

## Residual / Next Slices

- No enforcement: caps are declared and visible but not yet applied at dispatch.
  A runaway loop is still stopped by operator luck, not by `iteration_cap`.
- `provider_key_name` is a label only; no key is minted or looked up.
- `side_effect_policy` is stored but the kill/quarantine mechanism is unbuilt.
- Fixtures proving an infinite loop is stopped by code remain open (oracle).
