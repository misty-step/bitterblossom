# The event-plane spine (`bb`)

Bitterblossom v3 is one Rust binary, `bb`, with two personalities:

- `bb serve` — the plane: webhook ingress, cron scheduler, queue, dispatch.
- `bb <verb>` — the operator/agent CLI sharing the same core, so every
  workflow also runs as dispatch work from a terminal with no webhook.

Vocabulary: **reflex** work is standing and trigger-fired (webhook/cron);
**dispatch** work is deliberate, operator- or agent-initiated (`bb run`).
The model & auth policy below hangs on that distinction.

The product is the config surface. A workload is files, not Rust:

```
plane.toml                  # db path, ingress bind/token, notify webhook, global budget
agents/<name>.toml          # versioned agent binding: harness, model, flags
tasks/<name>/card.md        # lane card — the agent's entire context
tasks/<name>/task.toml      # agent binding, substrate, workspace, budgets, triggers
```

## plane.toml

```toml
db_path = ".bb/plane.db"          # default; created on demand (WAL mode)

[ingress]                         # used by `bb serve`
bind = "127.0.0.1:7077"

[notify]
webhook_url = "https://ntfy.sh/my-plane"   # state transitions only

[budget]
max_cost_per_day_usd = 25.0       # global daily ceiling, enforced pre-dispatch
```

## agents/<name>.toml

```toml
version = 1                       # bump on any change; recorded on every attempt
harness = "pi"                    # claude | codex | pi
model = "moonshotai/kimi-k2.6"
provider = "openrouter"           # pi only; defaults to "openrouter"
auth = "api"                      # api | subscription (defaults by harness)
bin = "pi"                        # optional: override the harness binary path
args = []                         # optional: extra CLI args appended verbatim
secrets = ["OPENROUTER_API_KEY"]  # env names resolved per-exec, never persisted
```

Swapping a task's agent is a one-line edit to `task.toml`; the ledger
records which agent name + version produced every attempt.

### Model & auth policy (enforced at load — `bb check` fails, not dispatch)

Two auth classes, two work classes:

- **`subscription`** (claude/codex default): the agent runs *as* the
  operator on OAuth subscription auth. API keys are forbidden —
  `ANTHROPIC_API_KEY`/`OPENAI_API_KEY` as agent secrets fail the load, as
  does `auth = "api"` on those harnesses. Subscription agents bind only
  to manual-only tasks (**dispatch** work).
- **`api`** (pi default): cheap open-weight models via OpenRouter. The
  only class allowed on webhook/cron triggers (**reflex** work). Execs
  are hermetic: scrubbed environment, workspace-local HOME, declared
  secrets only — nothing of the operator's identity crosses the exec
  boundary.

## tasks/<name>/task.toml

```toml
agent = "reviewer"                # agents/reviewer.toml
substrate = "sprites"             # remote-only; "local" needs plane dev = true

[workspace]                       # materialized by the substrate preparer
host = "bb-demo"                  # sprites: the sprite name (host lease key)
repos = [{ url = "https://github.com/o/r.git", ref = "master" }]

[budget]
timeout_minutes = 30              # enforced: wall-clock cancel
max_runs_per_day = 10             # enforced pre-dispatch
max_cost_per_run_usd = 2.0        # advisory in v1: breach parks the task
turn_cap = 50                     # enforced only where the harness streams turns

[[trigger]]
kind = "manual"                   # `bb run <task>`; the degenerate trigger

[[trigger]]
kind = "cron"
schedule = "0 */6 * * *"          # dedupe key = the scheduled timestamp

[[trigger]]
kind = "webhook"
route = "demo"                    # POST /hooks/demo
secret_env = "BB_HOOK_DEMO"       # HMAC-SHA256 secret env var
dedupe_key = "header:X-GitHub-Delivery"   # or "json:<pointer>"

# Containment filters (ANDed; fail-closed on missing pointers). An
# authenticated delivery failing any filter is acknowledged with 200
# but never becomes a run — scope lives in config, not card prose.
[[trigger.filter]]
pointer = "/repository/full_name"
any_of = ["misty-step/bitterblossom"]
[[trigger.filter]]
pointer = "/pull_request/draft"
equals = false
[[trigger.filter]]
pointer = "/pull_request/additions"
max = 4000

pre_command = ""                  # optional adapter commands run in the
post_command = ""                 # workspace before/after the agent
```

## Observability

The ledger is the system of record; everything reads from it:

- `GET /` — server-rendered HTML operator view (tasks, budgets, parked
  state, recent runs; auto-refreshes).
- `GET /api/runs[?task=&state=]`, `GET /api/runs/<id>` (run + attempts +
  events), `GET /api/dlq`, `GET /api/tasks` — the agent-facing read API,
  same shapes as the `--json` CLI.
- Auth: set `BB_API_TOKEN` on the plane and send
  `Authorization: Bearer <token>` (browsers may use `?token=`). Unset =
  open, acceptable only on the loopback default bind.

Cost attribution rides OpenRouter's per-response usage accounting
(`usage.cost` arrives with every pi response — no extra calls), parsed
per attempt into the ledger. Decision 2026-06-10: no OTel/Langfuse
sidecar for now — the OTel GenAI semantic conventions are still
experimental and both add infra the ≤5k LOC spine doesn't need; if
deeper traces are wanted later, `bb runs export` is the integration
seam (map attempts onto `gen_ai.*` spans then).

## The submission loop

Completed agent work is quality-assured and landed by a **verdict storm**
plus a **mechanical gate** — no human reads the code, no PR is the
channel. The spine holds the data mechanics only (submissions, verdicts,
gate arithmetic); what a reviewer looks for lives in cards.

**Submissions.** `bb submit open --change <key> --rev <sha>` creates a
submission: `open → clear | blocked | escalated | abandoned`, at most one
non-terminal submission per change key (CAS-enforced). The change key and
rev are opaque strings (branch + SHA today; jj change IDs later, zero
spine change). Round numbering is plane-owned: reopening after `blocked`
increments the round and snapshots the prior gate report — the driver
cannot soften or omit prior findings; verdict tasks receive the canonical
report as `REPORT.json` next to `EVENT.json`.

**Verdict tasks.** A task with `verdict = "<kind>"` in task.toml is a
storm member. Its payload is `{"submission": "<id>", ...}` and its parsed
result MUST be verdict JSON:

```json
{"verdict": "pass|blocking|advisory",
 "findings": [{"severity": "blocking|serious|minor",
               "file": "src/x.rs", "line": 42,
               "claim": "...", "evidence": "...",
               "fingerprint": "<from REPORT.json when re-raising, else omitted>"}]}
```

Unparseable verdict JSON fails the run, raw output preserved. The plane
fingerprints findings (`sha256(kind|file|claim)`) when absent. The
`command` harness maps an exit code to a verdict with no LLM (exit 0 →
`pass`, non-zero → one `blocking` finding carrying the stderr tail) — a
deterministic gate like `verify.sh` is never mediated by an agent.

**The gate.** `plane.toml` declares policy; `bb gate --change <key>`
evaluates pure data:

```toml
[gate]
required = ["verify", "correctness", "security", "simplification", "product"]
max_rounds = 3                    # rounds 1..=max_rounds may run
arbiter = "arbiter"               # verdict kind that settles disputes
```

- A required kind without a terminal run → `pending` (per-kind states
  listed). A required kind whose run is terminal `failure` → the
  submission **escalates** (one notify): infrastructure failure is loud,
  never an eternal pending. `clear` is only emitted over a complete round.
- **Only `blocking`-severity findings block — every round.** Fresh
  blockers are never demoted by recency; termination rests solely on the
  round cap. `serious`/`minor` never block (anti-needling is mechanical).
- A rejected fingerprint (`bb submit reject`) cannot block again — but
  rejecting a `blocking` finding only takes effect once an **arbiter**
  verdict independently sustains the rejection. Rejections and reasons
  appear verbatim in every subsequent report.
- `blocked` at `round == max_rounds` → `escalated` (one notify).

**The driver (convention, not spine).** On judging work complete: push
the branch → `bb submit open` → fire required storm members as parallel
`bb run <kind> --idempotency-key storm:<submission>:<kind> --payload
'{"submission":"<id>", ...}'` → `bb gate` (safe to call any time) → on
`clear`: file advisories to backlog.d, squash-land (`clear` is terminal);
on `blocked`: fix, push, `bb submit open` for the next round; on
`escalated`: stop — the operator is already notified. Judgment (what to
fix, what to reject) stays with the agent; arithmetic lives in `bb gate`.

## Run lifecycle

A durable run row exists in SQLite **before any trigger gets its ack**.
States: `pending → running → success | failure | awaiting_recovery`, plus
`blocked_budget` for ingress on a parked task (recorded, never dispatched,
until `bb task unpark`). Each dispatch attempt checkpoints its phase —
`acquired → prepared → executing → collecting → finalizing → released` —
because agent runs have external side effects and "re-run it" is not a
recovery semantic:

- Failures **before** `executing` retry mechanically (2 retries), then
  dead-letter with full payload + attempt history.
- Failures at or after `executing` go straight to `failure` or
  `awaiting_recovery`; replay is an explicit operator act.
- On boot, inherited `running` runs are classified by probing the host and
  reading attempt artifacts — never blindly orphaned.
- `bb dlq replay <id>` mints a **new** run linked via `parent_run_id`.

Host mutual exclusion is a durable lease keyed by substrate resource
identity (the sprite/host), not by task: two tasks sharing a host never
overlap. Per-task FIFO ordering is layered above that lease.

## Operator CLI

All read commands take `--json` and emit stable shapes (agents are users).

```
bb run <task> [--idempotency-key K] [--var k=v]   # manual trigger
bb runs list [--task T] [--state S] [--json]
bb runs show <run-id> [--json]                    # run + attempts + events
bb runs export [--since ...]                      # flat JSONL for Daedalus
bb dlq list|replay <id>
bb task park|unpark <task>
bb submit open --change K --rev SHA [--context TEXT]
bb submit reject --change K --fingerprint FP --reason TEXT
bb submit abandon --change K
bb gate --change K | --submission ID [--json]     # also GET /api/gate?change=K
bb serve                                          # webhook + cron + queue
```

Cost and tokens are parsed from harness output per attempt; unparseable
output is a `failure` with raw output preserved on the attempt row — never
a silent zero-cost success.

## What the plane refuses to know

No workload logic, no judgment. Retry semantics are mechanical; agents own
their own decomposition. If a feature needs a workload-specific branch in
dispatch/queue/substrate, it belongs in the task spec or in harness-kit.
Spine budget: ≤ ~5k LOC. Design rationale and critique record:
`docs/plans/2026-06-10-031-event-plane-spine.md`, ADR 005.
