# Shape and build the Rust event-plane spine

Priority: P0
Status: done
Estimate: XL

Context packet: `docs/plans/2026-06-10-031-event-plane-spine.md`
(shaped + critiqued 2026-06-10; consume via `/deliver`).

## Goal
A small Rust service + CLI that implements the v3 primitives — task, agent,
trigger, run, substrate — well enough to carry the first workload (028)
end to end.

## Scope (the spine, nothing else)

- Ingress: webhook route + cron scheduler + `bb run <task>` CLI, all
  writing a durable run row (SQLite) before acking.
- Queue: per-task serialization, retry wrapper, dead letters with replay.
- Substrate contract: prepare (sprite checkpoint restore + repo
  hard-reset), exec (harness CLI with lane card + per-exec credentials),
  collect (artifacts, receipts, cost), cancel. Fly Sprites adapter first;
  local-process adapter as the degenerate case.
- Task/agent/trigger as config + lane-card files, not code.
- Operator CLI with stable `--json`: runs list/show, dead-letter replay,
  budget burn.
- Telemetry: cost + tokens per run in the ledger; OTel/Langfuse-shaped
  export hook.

## Oracle
- [x] `/shape` produces a context packet (data model, substrate contract,
      crate layout) reviewed before implementation starts —
      `docs/plans/2026-06-10-031-event-plane-spine.md`, codex critique
      receipt e12ad318
- [x] A demo task defined purely in config runs end to end via all three
      ingress forms (webhook, cron, CLI) with one durable run row each —
      live QA 2026-06-10: webhook 202 + dedupe, two cron fires, manual;
      real codex run on sprite lane-1 (`BB-M2-OK`); tests/e2e_local.rs,
      tests/e2e_sprites.rs, tests/ingress.rs
- [x] Killing the service mid-run yields a classified (never silently
      lost) run — boot recovery probes the host: pre-execute dead-letters
      for mechanical replay, executing parks in awaiting_recovery; `bb dlq
      replay` mints a linked run (parent_run_id) — tests/recovery.rs +
      live dlq replay QA
- [x] Swapping the demo task's agent binding is one task.toml edit, both
      bindings visible in the ledger with name + version — tests/budgets.rs
- [x] Cost and duration per run queryable via `bb runs list/show --json`
      and `bb runs export` (JSONL) — live QA: $0.0421 / 309ms recorded
- [x] CI gate (030): .github/workflows/ci.yml — fmt, clippy -D warnings,
      test
- [x] Cross-model adversarial review: codex receipts 56b12e44 (8 findings,
      all fixed) + 471f9eae (4 residuals fixed, remaining holds verified)

## Notes
Study before shaping: olympus `orchestrator/src/index.ts` +
`execution-substrate.ts` (the lane/substrate seam), `inngest/utah` (durable
agent loop shape), harness-kit `skills/sprites/references/provisioning.md`
(golden-checkpoint lifecycle the Elixir conductor already learned the hard
way). Absorbs the intents of abandoned tickets 022 (cost/model routing),
023 (harness-agnostic agent binding), 027 (workspace freshness = substrate
prepare contract). The 20K-LOC Python conductor postmortem applies: shape
first, keep the spine under a few thousand lines, no judgment in the plane.

Loop guardrails are a spine contract, not a per-task nicety (harness-kit
`meta/CONTRACTS.md` §6, 2026-06-10): every unattended loop names max
iterations, no-progress detection, and a token/dollar budget before it
runs; a halted loop files what it found and stops. The budget-burn CLI and
retry/dead-letter scope above are where these land.
