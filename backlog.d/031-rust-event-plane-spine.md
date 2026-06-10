# Shape and build the Rust event-plane spine

Priority: P0
Status: pending
Estimate: XL

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
- [ ] `/shape` produces a context packet (data model, substrate contract,
      crate layout) reviewed before implementation starts
- [ ] A demo task defined purely in config runs end to end via all three
      ingress forms (webhook, cron, CLI) with one durable run row each
- [ ] Killing the service mid-run yields an `orphaned`/recoverable run, not
      silent loss; a failing dispatch dead-letters after bounded retries
      and is replayable from the CLI
- [ ] Swapping the demo task's agent binding (e.g. codex → claude) requires
      only a config change and is visible in the run ledger
- [ ] Cost and duration for each run are queryable via `bb runs --json`

## Notes
Study before shaping: olympus `orchestrator/src/index.ts` +
`execution-substrate.ts` (the lane/substrate seam), `inngest/utah` (durable
agent loop shape), harness-kit `skills/sprites/references/provisioning.md`
(golden-checkpoint lifecycle the Elixir conductor already learned the hard
way). Absorbs the intents of abandoned tickets 022 (cost/model routing),
023 (harness-agnostic agent binding), 027 (workspace freshness = substrate
prepare contract). The 20K-LOC Python conductor postmortem applies: shape
first, keep the spine under a few thousand lines, no judgment in the plane.
