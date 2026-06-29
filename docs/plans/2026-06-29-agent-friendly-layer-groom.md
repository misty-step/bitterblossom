# Agent-Friendly Layer Groom

Date: 2026-06-29

## Goal

Lock the 2026-06-29 Bitterblossom architecture discussion into backlog shape: keep the plane thin, prioritize the agent-facing skill/CLI/MCP layer, and stage unsupervised workflows through evidence-gated authority levels.

## Source Matrix

| Source | Evidence | Contribution |
|---|---|---|
| Operator premise | Discord voice/text transcript saved in Hermes document cache plus served HTML artifact | Prioritize research/design/implementation of the agent-friendly layer; BB should become the tool of choice for unsupervised agent workflows and cron jobs. |
| Bitterblossom repo vision | `VISION.md`, `project.md`, `docs/spine.md`, `AGENTS.md` | Confirms thin event-plane boundary: mechanics in Rust, judgment in task cards and agents. |
| Existing skill/interface | `skills/bitterblossom/`, `tests/skill_artifacts.rs`, `tests/cli_contract_docs.rs` | Skill exists; remaining work is schema-backed contract, authority tiers, CLI help, MCP, and artifact access. |
| Harness Kit Mode boundary | `/Users/phaedrus/Development/harness-kit/meta/CONTRACTS.md` | Mode A stays Harness Kit; Mode B event/cron/webhook loops belong to Bitterblossom and must also run ad hoc from terminal. |
| Harness Kit loop readiness | `harnesses/shared/references/loop-readiness.md` | Loops require repetition, external verifier, reproducible environment, and hard budgets. |
| Harness Kit model-native boundary | `harnesses/shared/references/model-native-product-primitives.md` | Avoid over-structuring model-facing context; impose schema only where deterministic code branches or contracts cross components. |
| Served artifact | `https://serenity.tail5f5eb4.ts.net/artifacts/a/CcRplMnv_gpNrtFTlRTCncc_/` | Human-readable architecture packet for this groom; backlog is the durable execution surface. |
| Follow-up swarm: first pickup | Async lane inspected live repo/backlog/source/tests | Confirmed `077` plus a narrow first slice of `053` is the right implementation start; defer `078/079/083` until local JSON surfaces are boring. |
| Follow-up swarm: dogfood/runtime | Async lane ran repo-local `./target/debug/bb` read-only/preflight commands against `plane/` | Current plane has 30 Sprite-backed tasks, 200 runs, 10 open DLQs, no zero-credential local plane; read-only 053 inventory is safe, mutating dispatch is paid/auth-gated. |
| Follow-up swarm: portfolio factory | Async lane lightly inspected BB/Canary/Cerberus/Crucible/Harness Kit/Landmark repos | First cross-repo factory loop should be Canary incident → BB report-only triage → artifact-backed next action; backlog-chewer comes second. |

## World-Class Plan

Bitterblossom should become the durable event/control/observability plane that agents use when work is remote, recurring, event-triggered, or scheduled. The product surface is:

1. **Skill** — model-facing operating doctrine, authority tiers, Mode B readiness, and closeout receipts.
2. **CLI** — canonical local/operator/script contract with stable JSON, good help, payload validation, and artifact access.
3. **MCP** — typed agent adapter over the canonical CLI/API, read-only first and mutating only behind explicit authority modes.
4. **Workflows** — Canary triage and backlog-chewer loops, staged through report-only and PR-only modes before any merge/revert authority.

The over-engineering line is clear: add contracts and observability around the plane, not workflow judgment inside it.

## Backlog Diff

Created:

- `076-agent-friendly-layer-v1.md` — P0 epic for skill + CLI + MCP + artifacts.
- `077-zero-credential-local-plane-and-cli-baseline.md` — P0 cold-start and payload validation.
- `078-read-only-mcp-server.md` — P1 read-only MCP adapter.
- `079-artifact-cli-and-mcp-resources.md` — P1 artifact inspection surface.
- `080-canary-triage-report-only-workflow.md` — P1 first Canary incident workflow.
- `081-canary-remediation-authority-ladder.md` — P2 staged branch/merge/revert authority, gated by evidence.
- `082-backlog-chewer-dry-run-and-pr-only-workflows.md` — P2 whitelisted backlog automation, dry-run then PR-only.
- `083-unattended-loop-safety-guardrails.md` — P1 pause/caps/outbox/heartbeat/reservation guardrails.

Updated:

- `053-versioned-agent-contract-and-skill-projection.md` — re-promoted from P2 to P0 and tied to epic 076.

Related existing tickets kept by reference:

- `051-deterministic-recovery-and-probe-contract.md` — recovery/probe determinism.
- `055-workload-template-portfolio.md` — copyable starter templates.
- `058-durable-workflow-build-vs-borrow-recheck.md` — substrate/workflow system bakeoff.
- `062-remaining-lifecycle-reflexes.md` — SDLC reflex breadth.
- `066-operator-api-shape-consistency.md` — API shape consistency.
- `072-plane-observability-core-and-dashboard.md` — core read surface and dashboard.
- `073-dispatch-readiness-for-subscription-builders.md` — manual builder readiness.

## Recommended Sequence

1. `077` + `053`: local golden path, payload validation, fixture-backed JSON contracts. Keep `053` narrow: lock the surfaces `077` exercises before full schemas/projection.
2. `078`: read-only MCP over the canonical read surfaces.
3. `079`: artifact read/list so agents can inspect evidence.
4. `083` minimum safety slice: pause/caps/status/budget containment before live webhook or cron volume.
5. `080`: Canary report-only triage, starting with a manual/replayable fixture before live webhook ingress.
6. `082`: backlog-chewer dry-run.
7. `081`: only after report-only and PR-only workflows prove themselves.

## Residual Risk

- Canary-side API/schema requirements need a focused read of `/Users/phaedrus/Development/canary` before implementing `080`; the portfolio lane found `canary/backlog.d/010-ramp-pattern.md` as the current north-star reference and `048-responder-rich-context-safety-gate.md` as the expansion gate.
- MCP implementation details should be re-shaped before coding if the Rust dependency choice or protocol surface is unclear.
- Harness Kit git inspection in the portfolio lane hit filesystem/resource errors and an unexpected toplevel path, so Harness Kit evidence here is skill/doctrine-based rather than repo-status-based.
- The current live BB plane is useful for read-only contract dogfood, but all current tasks are Sprite-backed and preflight shows missing secrets in this shell; do not treat it as a zero-credential proof.
