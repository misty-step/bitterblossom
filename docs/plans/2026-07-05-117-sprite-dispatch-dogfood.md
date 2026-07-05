# bitterblossom-117 — Sprite-Backed Dogfood Proof for Ad Hoc Dispatch

Lane: `lane-bb-trio`. Status: proof complete, temporary config removed.

## Goal

Prove the ad hoc dispatch machinery bitterblossom-116 shipped (the opt-in
MCP `bb_dispatch` tool, sharing `dispatch::build_dispatch_job_payload` with
the CLI `bb dispatch` command) end to end: a real Sprite-backed run,
off-laptop, with duplicate-active-work refusal and its explicit force path,
all against the resident `plane/` directory this machine already uses for
production reflex tasks (canary-triage, self-drill). The job itself is
trivial and read-only on purpose -- the point is the dispatch machinery, not
the job.

## Preflight

- Sprites org/host reachable: `sprite org list` showed `misty-step` selected;
  `sprite list -o misty-step` included `lane-1` (the host every existing
  `plane/` task already targets); `sprite -o misty-step -s lane-1 exec --
  whoami` returned `sprite`.
- `plane/` is gitignored operator instance config (`/plane/` in
  `.gitignore`), not tracked in the product repo -- safe to add and remove a
  temporary task without touching git at all.
- `plane/` had no `dispatch`/`build` ad hoc task and two existing manual
  tasks (`canary-triage`, `self-drill`), so `default_dispatch_task` would
  refuse ambiguously without an explicit task. Added one temporary task
  rather than repurposing either existing production reflex task or its
  daily/weekly budget quota.

## Temporary Task (added, then removed)

`plane/tasks/dispatch-demo/task.toml` -- `substrate = "sprites"`, host
`misty-step/lane-1`, one repo (`misty-step/bitterblossom.git@master`,
matching the existing tasks' workspace shape), `timeout_minutes = 5`,
`max_runs_per_day = 5`, `max_cost_per_run_usd = 0.01`, one manual trigger.

`plane/agents/dispatch-demo.toml` -- deterministic `command` harness (`bin =
"sh"`, matching `self-drill-runner.toml`'s pattern, zero model cost): prints
the cloned repo's current `HEAD` commit (`git log -1 --oneline`) and writes
`REPORT.json`. Read-only; no push, no branch, no PR.

`GH_TOKEN=$(gh auth token) ./target/debug/bb --config plane check` loaded all
three tasks cleanly. `bb --config plane preflight dispatch-demo --json`
returned zero findings before any Sprites spend.

## Dispatch (dogfooding bitterblossom-116's MCP tool)

Started `bb --config plane serve` in the background (binds
`127.0.0.1:7077`, no public exposure) so the enqueued run would actually
drain onto the Sprite -- `bb dispatch`/`bb_dispatch` only enqueue; a running
`bb serve` executes.

Dispatched through the opt-in MCP tool itself, not the CLI, per the lead's
steer that using 116's own tool for 117's proof is the stronger dogfood:

```bash
printf '%s\n' '{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"bb_dispatch","arguments":{"repo":"'"$PWD"'","prompt":"bitterblossom-117 sprite-backed ad hoc dispatch dogfood proof...","label":"bb-117-dogfood"}}}' \
  | BB_MCP_ENABLE_DISPATCH=1 BB_DISPATCH_TASK=dispatch-demo ./target/debug/bb --config plane mcp serve
```

Result: `run_id=c8b9d3648dab`, `task=dispatch-demo`, `state=pending`,
`duplicate=false`, `idempotency_key=mcp-dispatch:bb-117-dogfood:e9da808f1aff1e75`.

## Live Run Evidence

`BB_LOGS_POLL_MS=500 bb --config plane logs -f c8b9d3648dab`:

```text
2026-07-05T04:43:13.960459Z state:running
2026-07-05T04:43:18.346195Z progress phase:prepared
2026-07-05T04:43:18.346464Z progress phase:executing
2026-07-05T04:43:18.767596Z progress phase:collecting
2026-07-05T04:43:18.768044Z progress phase:finalizing
2026-07-05T04:43:19.429464Z progress phase:released
2026-07-05T04:43:19.430234Z state:success
--- attempt 1 stdout.txt ---
0a2375a feat(operator): close dashboard data completeness against existing read APIs
terminal state=success
```

That commit line is the real `HEAD` of the repo as cloned on `lane-1`,
proving the job executed off-laptop, not locally.

`bb --config plane runs show c8b9d3648dab --json` (trimmed): `harness:
"command"`, `agent_name: "dispatch-demo"`, `cost_usd: null` (deterministic
command harness, no LLM spend), `tokens_in/tokens_out/turns: null`,
`duration_ms: 5469`, `state: "success"`.

Artifacts read through the public surface (`bb artifacts list/read`, not
local path spelunking): `stdout.txt`, `result.md`, `LANE_CARD.md`,
`REPORT.json`, `EVENT.json`, `RUN.json`, `stderr.txt` (empty).
`bb artifacts read c8b9d3648dab REPORT.json --json` returned
`{"status":"ok","note":"bitterblossom-117 ad hoc dispatch dogfood proof"}`.
No branch or PR was created -- the job was read-only by design, so none was
expected.

## Duplicate-Refusal Proof

Re-dispatched the identical `(repo, label, branch_slug)` through the same
opt-in MCP tool, no `force`:

```json
{"duplicate": true, "run_id": "c8b9d3648dab", "state": "success", "task": "dispatch-demo", "idempotency_key": "mcp-dispatch:bb-117-dogfood:e9da808f1aff1e75"}
```

Same run id as the original -- refused, not fanned out, even though the
first run had already reached a terminal state by the time of the second
call. `Ledger::ingest`'s `(task, idempotency_key)` uniqueness has no
active-state carve-out, so it is conservative in the safe direction: it
refuses *any* repeat of the same key, not only a still-running one.

## Explicit Force-Path Proof

Re-dispatched the same tuple with `"force": true`:

```json
{"duplicate": false, "run_id": "2989ce57fb9b", "state": "pending", "task": "dispatch-demo", "idempotency_key": null}
```

A genuinely new run id. Followed to completion the same way:
`state:success`, `duration_ms: 4297`, identical `stdout.txt` content
(the repo's `HEAD` had not moved between the two dispatches).

`bb --config plane runs list --task dispatch-demo --json` after both proofs
shows exactly two rows -- `c8b9d3648dab` and `2989ce57fb9b` -- confirming the
refused duplicate call never created a third.

## Cleanup

Stopped the background `bb serve` process. Removed
`plane/tasks/dispatch-demo/` and `plane/agents/dispatch-demo.toml` via
`/usr/bin/trash`. `bb --config plane check` re-run afterward shows only the
original `canary-triage` and `self-drill` tasks -- `plane/` is restored to
its pre-proof state. `plane/` is gitignored, so none of this touched git.

## Friction

None new. The dogfood ran cleanly on the first attempt: Sprites
provisioning, the MCP dispatch tool, duplicate refusal, and the force path
all behaved exactly as designed and documented in
`docs/mcp-dispatch-authority.md`. The one pre-existing, already-acknowledged
limitation this proof brushed against -- ad hoc dispatch has no per-call
`task` selector and depends on server-side `BB_DISPATCH_TASK` when a plane
has more than one manual task -- was explicitly waived in the original
bitterblossom-906 report ("a separate `--task` flag would expand the
requested command surface; use `BB_DISPATCH_TASK` for explicit selection")
and is not new friction from this lane.

## Verification

```bash
./scripts/verify.sh
```

Only docs changed for this card (this file); no source touched, so the gate
was re-run to confirm the repo state is still green, not because dispatch
mechanism itself changed.
