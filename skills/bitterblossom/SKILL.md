---
name: bitterblossom
description: |
  Operate Bitterblossom's `bb` event-plane CLI for agent workloads. Use when
  Codex needs to inspect or run `bb`, configure `plane.toml`, `agents/`, or
  `tasks/`, dispatch or audit tasks, handle runs, dead letters, recovery,
  parked tasks, submissions, gates, review-factory workflows, or help another
  repo consume Bitterblossom. Trigger phrases: "Bitterblossom", "bb",
  "event plane", "agent workload", "run a task", "inspect runs", "DLQ",
  "parked task", "submission loop", "review factory".
---

# Bitterblossom

Operate the event plane. Do not move workload judgment into the plane.

Bitterblossom is `bb`: tasks + agents + triggers as files, with a durable run
ledger, budgets, retries, dead letters, and optional webhook/cron serving.
Agents are CLI users, so prefer stable `--json` surfaces over prose parsing.

## Stance

- Treat `bb` as runtime and ledger, not an agent brain.
- Workloads are config: `plane.toml`, `agents/<name>.toml`,
  `tasks/<name>/task.toml`, and `tasks/<name>/card.md`.
- New workload behavior should be a task/card/agent change, not a Rust branch.
- Use `--config <plane>` explicitly unless the user has set `BB_PLANE_DIR`.
- Use `--json` for agent-readable output. Text is for humans.
- Human-mode `bb run` prints an early run id and periodic stderr heartbeats;
  `bb run --json` stays quiet until it emits the final run bundle.
- If `bb` is not on `PATH`, use `cargo run --quiet --` from the source repo or
  `./target/debug/bb` after `cargo build`.

## First Probe

Run these before changing or dispatching anything:

```bash
bb --config <plane> check
bb --config <plane> task list --json
bb --config <plane> runs list --json
```

Read the output for:

- loaded tasks and agent versions;
- parked tasks and budget ceilings;
- recent failures, dead letters, and costs;
- whether a task is reflex (`webhook`/`cron`) or dispatch (`manual`).

## Route

| Need | Use |
|---|---|
| Validate config and see loaded agents/tasks | `bb --config <plane> check` |
| Decision-ready task/run/DLQ health | `bb --config <plane> status --json` |
| Task inventory, parked state, budgets | `bb --config <plane> task list --json` |
| Trigger manual work | `bb --config <plane> run <task> --payload '<json>' --json` |
| Inspect ledger | `bb --config <plane> runs list --json`; `bb --config <plane> runs show <id> --json` |
| Export run telemetry | `bb --config <plane> runs export` |
| Handle pre-execute failures | `bb --config <plane> dlq list --json`; `bb --config <plane> dlq replay <id> --json` |
| Park or unpark workload dispatch | `bb --config <plane> task park|unpark <task>` |
| Classify inherited running rows after host restart | `bb --config <plane> recover` |
| Run webhook/cron plane | `bb --config <plane> serve` |
| Submission storm / review factory | `bb submit ...`, verdict `bb run <kind> ...`, then `bb gate --json` |

Detailed command recipes: `references/operator-recipes.md`.

## Dispatch Rules

- A `bb run` can have external side effects. Do not blindly re-run a successful
  or executing task.
- Secrets travel through declared env/secrets and stdin plumbing. Never put
  tokens in argv, task cards, or payload JSON unless the task contract explicitly
  says the value is non-secret.
- For GitHub-backed runs, prefer `GH_TOKEN=$(gh auth token) bb ...` over copying
  tokens into shell history.
- Reflex triggers must use API-auth agents. Subscription-auth agents belong to
  manual dispatch only.
- A parked task is intentionally blocked. Inspect the reason before `unpark`.
- Dead letters are pre-execute failures. At/after execute, use operator
  resolution paths because the run may have side effects.

## Submission Storm

Verdict tasks (`correctness`, `security`, `product`, `simplification`,
`arbiter`, `verify`) expect a submission payload. Do not call them with an
arbitrary repo/rev payload.

Shape:

```bash
bb --config <plane> submit open --change <change> --rev <rev> --json
bb --config <plane> run correctness --payload '{"submission":"<submission>"}' --json
bb --config <plane> gate --submission <submission> --json
```

If a verdict task fails with `payload has no 'submission' field`, the plane is
correct; the invocation was wrong.

If `bb gate --json` reports a canonical member as `run:failure`, read that
member's `safe_next_command`. Replays prove the failed pre-execute path can run,
but they do not count for the canonical gate key; the safe recovery is a clean
replacement submission after fixing the operator or infrastructure issue. The
command includes the loaded plane's `--config` path so an agent can run it from
another cwd.

## Recovery

- `bb recover` classifies inherited `running` rows after a host restart.
- `bb runs resolve` is for `awaiting_recovery` after side-effect inspection.
- `bb status --json` marks `awaiting_recovery` older than one hour as
  `escalate_stale_recovery`, with age fields, but the operator still resolves
  the run only after inspecting side effects.
- `bb dlq replay --json` mints a new run linked to a pre-execute dead letter
  and returns the replayed run bundle.
- There may be no "acknowledge this intentional failed probe" command yet;
  record the run id and reason in closeout instead of hiding it.

## Serving

`bb serve` exposes webhook ingress, cron scheduling, the HTML operator view, and
read APIs. Do not bind publicly without `BB_API_TOKEN`; the server refuses
non-loopback binds without it.

Useful API mirrors:

- `GET /api/tasks`
- `GET /api/status`
- `GET /api/runs`
- `GET /api/runs/<id>`
- `GET /api/dlq`
- `GET /api/submissions`

## Distribution

The portable artifact is this whole folder: `skills/bitterblossom/`. Consumers
should copy or symlink the folder, not just `SKILL.md`, so references and agent
metadata travel with it.

Harness Kit integration should keep one source of truth. Prefer a source entry,
bootstrap projection, or explicit symlink from Harness Kit to this folder over a
manual copied skill that can drift.

## Closeout Evidence

When using Bitterblossom, report:

- exact plane path and `bb` binary used;
- commands run and relevant run ids;
- ledger state, costs, parked/DLQ status, and external side effects;
- generated artifacts or API/CLI JSON read;
- residual risk, including failed probes that remain in the ledger.
