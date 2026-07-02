# Ship the agent-friendly Bitterblossom layer v1

Priority: P0 · Status: done · Estimate: XL

## Goal

Make Bitterblossom the tool an agent reaches for when it needs durable, remote, event-triggered, or scheduled work: a packaged skill, self-teaching CLI, read-first MCP surface, and artifact evidence path over the same thin event-plane spine.

## Oracle

- [x] A credential-free local quickstart proves a first successful run without Sprites, OpenRouter, GitHub, or Fly credentials.
- [x] Agent-facing JSON surfaces used by skill/MCP are fixture-backed and compatibility-tested: `check`, `status`, `task list`, `runs list/show`, `dlq list`, `gate`, and the matching `/api/*` routes.
- [x] `skills/bitterblossom/` carries an authority table for supervised vs unsupervised agents, Mode B readiness checks, and a strict closeout receipt contract.
- [x] `bb mcp serve` exposes read-only tools for status, check, tasks, runs, DLQ, gate, preflight, and artifact inspection, all backed by the same shapes as CLI/API.
- [x] Mutating MCP tools are explicitly deferred or guarded by mode, allowlist, reason, idempotency key, and confirmation.
- [x] Artifact inspection is first-class enough that an agent can read `REPORT.json` and logs without path archaeology.
- [x] `./scripts/verify.sh` passes.

## Verification System

- Claim: a cold consuming agent can inspect, understand, and safely invoke Bitterblossom through skill, CLI, or MCP without relying on prose-only tribal knowledge.
- Falsifier: a skill recipe references stale CLI flags; MCP returns a shape different from CLI/API; a cold run needs external credentials; or an agent cannot inspect the resulting artifact.
- Driver: `examples/local-plane` smoke run, CLI fixture tests, MCP smoke test, and artifact read test.
- Grader: fixture compatibility checks plus a fresh agent/harness consumer following only the skill and command help.
- Evidence packet: command transcript for local-plane + MCP smoke + artifact read, linked from a doc or PR.
- Cadence: every agent-interface change extends the same contract tests before shipping.

## Children

1. **Credential-free local plane** — see backlog 077.
2. **Versioned machine contracts** — continue and reprioritize backlog 053 plus 066 for shape consistency.
3. **CLI help and payload validation** — include in backlog 077 or split if it grows.
4. **Read-only MCP** — see backlog 078.
5. **Artifact CLI/MCP resources** — see backlog 079.
6. **Guarded mutating MCP** — deferred until read-only MCP and artifacts are boring.

## Notes

Why: the 2026-06-29 architecture groom corrected the priority. The portable skill already exists (`skills/bitterblossom/`), but the product surface is the combined skill + CLI + MCP + artifact path. This is not over-engineering; it is the agent consumption layer. The over-engineering risk is putting judgment, merge authority, provider orchestration, or workflow semantics into the Rust spine instead of task cards and agents.

Thin-plane invariant: Bitterblossom owns event ingress, dedupe, queue, run ledger, budgets, workspace materialization, harness execution, artifacts, notifications, recovery, and observability. It does not decide product direction, pick backlog items by vibes, or grade its own work.

## Delivery Notes

### 2026-07-02 closure slice

- Backlog 077 delivered the zero-credential `examples/local-plane` quickstart.
  This closure slice tightened `scripts/verify.sh` so the golden path parses run
  counts as JSON, captures the successful run id, checks `runs show`, and reads
  `REPORT.json` through `bb artifacts read`.
- Backlog 053/066 delivered fixture-backed CLI/API read contracts for agent
  surfaces, including `task list`, `runs list/show`, `dlq list`, `gate`, and
  `/api/*` mirrors.
- Backlog 078/079 delivered read-only MCP tools for status, check, tasks, runs,
  dead letters, preflight, gate, and artifacts, with tests comparing MCP output
  to CLI JSON.
- `skills/bitterblossom/SKILL.md` now carries the missing authority/readiness
  table for supervised dispatch, unsupervised reflex, and read-only inspection,
  plus a strict closeout receipt contract.
- Mutating MCP remains explicitly refused by the read-only server. Future write
  surfaces require a separate backlog with mode, allowlist, reason,
  idempotency, and confirmation gates.
