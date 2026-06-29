# Ship the agent-friendly Bitterblossom layer v1

Priority: P0 · Status: ready · Estimate: XL

## Goal

Make Bitterblossom the tool an agent reaches for when it needs durable, remote, event-triggered, or scheduled work: a packaged skill, self-teaching CLI, read-first MCP surface, and artifact evidence path over the same thin event-plane spine.

## Oracle

- [ ] A credential-free local quickstart proves a first successful run without Sprites, OpenRouter, GitHub, or Fly credentials.
- [ ] Agent-facing JSON surfaces used by skill/MCP are fixture-backed and compatibility-tested: `check`, `status`, `task list`, `runs list/show`, `dlq list`, `gate`, and the matching `/api/*` routes.
- [ ] `skills/bitterblossom/` carries an authority table for supervised vs unsupervised agents, Mode B readiness checks, and a strict closeout receipt contract.
- [ ] `bb mcp serve` exposes read-only tools for status, check, tasks, runs, DLQ, gate, preflight, and artifact inspection, all backed by the same shapes as CLI/API.
- [ ] Mutating MCP tools are explicitly deferred or guarded by mode, allowlist, reason, idempotency key, and confirmation.
- [ ] Artifact inspection is first-class enough that an agent can read `REPORT.json` and logs without path archaeology.
- [ ] `./scripts/verify.sh` passes.

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
