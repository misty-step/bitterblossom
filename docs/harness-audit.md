# Harness audit: opencode as the default open harness (bitterblossom-935)

## Doctrine

`opencode` is the default harness for new open-harness (`auth = "api"`) bb
agents. Flip to `pi` only when the workload needs
`policy.turn_cap`/`tool_action_cap`/`iteration_cap` enforcement: opencode has
no CLI flag for any of the three today, so `build_command` refuses to
dispatch a capped opencode agent rather than run one unbounded
(`supports_turn_cap`/`supports_tool_action_cap` in `src/harness.rs`). `omp`
remains available for workloads that specifically need its command surface.
`claude`/`codex` are unaffected — they run dispatch (manual) work on
subscription auth and are a separate class from the open/reflex harnesses
this audit covers.

Revisit this doctrine (and the capped-workload dispositions below) once
opencode ships a native per-step or per-turn budget flag.

## Audit table

Every agent bound to a task in this repo (production `plane/` + the
copyable `examples/` templates), current harness, and disposition.

| Task | Agent | Harness (before) | Caps declared | Disposition | Why |
|---|---|---|---|---|---|
| `plane/tasks/ci-audit-dogfood` | ci-audit-dogfood | pi | none | **Flipped -> opencode** | Temporary dogfood agent (bitterblossom-122), no turn/tool-action cap dependency; live smoke-tested. |
| `plane/tasks/canary-triage` | canary-triager | pi | `turn_cap = 40` (task budget) | Keep pi | Task-level turn cap is load-bearing containment for a webhook-triggered incident reflex; opencode can't enforce it pre-execution. |
| `plane/tasks/self-drill` | self-drill-runner | command | n/a | Keep command | Deterministic shell script, no model involved. |
| `examples/demo-plane/tasks/demo` | opencode (was pi) | pi | none | **Flipped -> opencode** | Illustrative reference with no policy caps; now the default-harness demo. `pi.toml` kept as the documented exception-path reference. |
| `examples/local-plane/tasks/hello-opencode` (new) | opencode-hello | — | none | New | Added as a zero-infra live smoke-test lane for the opencode harness end to end through `bb run` (see Evidence below). |
| `examples/local-plane/tasks/hello` | local-command | command | n/a | Keep command | Zero-credential command-harness reference, no model involved. |
| `examples/ci-audit-plane/tasks/ci-audit` | ci-auditor | pi | `turn_cap=35, tool_action_cap=70, iteration_cap=18` | Keep pi | Agent policy caps are the containment story for this reference template. |
| `examples/ci-audit-plane/tasks/ci-audit-pr` | ci-hardener | pi | `turn_cap=25, tool_action_cap=50, iteration_cap=12` | Keep pi | Same — edit-authority agent, caps are non-negotiable containment. |
| `examples/docs-sync-plane/tasks/docs-sync` | docs-watcher | pi | `turn_cap=35, tool_action_cap=70, iteration_cap=18` | Keep pi | Same. |
| `examples/docs-sync-plane/tasks/docs-sync-pr` | docs-sync-writer | pi | `turn_cap=25, tool_action_cap=50, iteration_cap=12` | Keep pi | Same — edit-authority agent. |
| `examples/review-factory-plane/tasks/review` | reviewer | pi | `turn_cap=40, tool_action_cap=80, iteration_cap=24` | Keep pi | Same. |
| `examples/powder-ready-plane/tasks/dispatch-ready` | dispatch-ready-builder | pi | `turn_cap=50, tool_action_cap=100, iteration_cap=24` | Keep pi | Same. |
| `examples/canary-responder-plane/tasks/incident-response` | incident-responder | pi | `turn_cap=40, tool_action_cap=80, iteration_cap=20` | Keep pi | Same. |
| `examples/roster-cerberus-plane/tasks/review` | cerberus | roster-materialized | `turn_cap=20, tool_action_cap=80` (task budget) | Keep (roster-owned) | Harness resolves from the pinned `vendor/roster` cerberus declaration, out of this repo's scope; task-level caps also block a flip regardless. |
| `examples/hygiene-plane/tasks/branch-prune` | branch-pruner | command | n/a | Keep command | No model. |
| `examples/hygiene-plane/tasks/dependabot-triage` | dependabot-triager | command | n/a | Keep command | No model. |
| `examples/moments-plane/tasks/moment-scorer` | moment-scorer | command | n/a | Keep command | No model. |
| `examples/demo-plane` (agent-only) | claude, codex | claude, codex | n/a (subscription) | Keep | Subscription-auth dispatch harnesses, a separate class from the open/reflex harnesses this audit covers. |

## Evidence

Live smoke run through the real `bb run` dispatch pipeline (prepare ->
exec opencode -> parse_output -> ledger), local substrate, zero infra cost:

```
bb --config examples/local-plane run hello-opencode --payload '{"ok":true}' --json
```

Run `ce751ece426e`, agent `opencode-hello@v1`, harness `opencode`, model
`deepseek/deepseek-v4-flash`: `outcome=success`, `cost_usd=0.001508958`,
`tokens_in=13737`, `tokens_out=152`, `turns=2`,
`result.md` = `BB-OPENCODE-LOCAL-OK` (the card's exact reply token).

The production flip (`plane/agents/ci-audit-dogfood.toml`, pi -> opencode)
gets its live-fleet smoke run as part of bitterblossom-122's resumed
docs-sync/ci-audit dispatch clock (same temporary agent, same task class).

## Known gap

`opencode` has no native flag for turn, iteration, or tool-action caps
(`opencode run --help` as of v1.2.6). `supports_turn_cap`/
`supports_tool_action_cap` in `src/harness.rs` return `false` for
`"opencode"`, so `build_command` bails before dispatch if a budget or
agent policy requests either — this is enforced, not just documented (see
`opencode_command_rejects_turn_cap_before_build` in `tests/harness.rs`).
Every capped reference workload above stays on `pi` until that gap closes.
