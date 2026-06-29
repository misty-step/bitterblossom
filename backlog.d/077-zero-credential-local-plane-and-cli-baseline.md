# Add a zero-credential local plane and self-teaching CLI baseline

Priority: P0 · Status: ready · Estimate: M

## Goal

Give cold humans and agents a one-minute Bitterblossom golden path that validates the plane, dispatches a task, records a run, exposes status, and reads artifacts without any external credentials or remote substrate.

## Oracle

- [ ] `examples/local-plane/` exists with `dev = true`, `substrate = "local"`, a `command` harness, and a `hello` or `report` task that writes a small `REPORT.json`.
- [ ] README quick start uses `examples/local-plane` first; Sprites/OpenRouter demo moves to the next section.
- [ ] These commands pass from a clean checkout with no BB-related secrets: `bb --config examples/local-plane check --json`, `bb --config examples/local-plane preflight hello --json`, `bb --config examples/local-plane run hello --payload '{"ok":true}' --json`, `bb --config examples/local-plane status --json`, and `bb --config examples/local-plane runs show <id> --json`.
- [ ] CLI help for every top-level command and high-use subcommand states purpose, side-effect level, JSON behavior, and common examples.
- [ ] `bb run --payload` validates JSON before creating a run row; invalid JSON exits non-zero with no new run.
- [ ] A `--payload-file` path exists or is explicitly deferred with a shaped follow-up.
- [ ] `./scripts/verify.sh` passes.

## Verification System

- Claim: a new agent can learn and exercise BB locally without external auth or hidden state.
- Falsifier: first run needs `OPENROUTER_API_KEY`, Sprites, GitHub, Fly, or accepts malformed `EVENT.json` into the ledger.
- Driver: unset BB/GitHub/OpenRouter/Sprite env vars in a subprocess, run the quickstart command sequence, then inspect run/artifact/status JSON.
- Grader: commands return expected JSON, no external network/auth is required, and invalid payload leaves run count unchanged.
- Evidence packet: quickstart transcript plus before/after run count under `.evidence/` or a docs plan.
- Cadence: README quickstart and CLI contract tests run in `./scripts/verify.sh`.

## Notes

Why: the existing `examples/demo-plane` is valuable but not a cold-start path because it uses `pi`/Sprites/OpenRouter. That is acceptable as a production-shaped demo, but not as the first agent-consumption proof.

This ticket should stay thin. It is not a new workflow. It is the local proof surface for every later skill/MCP/docs claim.

Swarm evidence 2026-06-29: the current `plane/` has 30 tasks and all are Sprite-backed; `preflight build`, `build-glm`, `build-kimi`, and `gardener` report missing `OPENROUTER_API_KEY` / `GH_TOKEN` in this shell, and `gardener` also wants `BB_API_TOKEN`. Use repo-local `./target/debug/bb` in verification because host `bb` may resolve to `/opt/homebrew/bin/bb` rather than the checked-out binary.
