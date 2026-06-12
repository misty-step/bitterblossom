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

Target repos may also own task definitions under `.bb/`, while the plane
keeps the trust boundary:

```
target-repo/.bb/tasks/<name>/card.md
target-repo/.bb/tasks/<name>/task.toml
```

The plane opt-in is explicit:

```toml
[[workload_repo]]
name = "target"                   # tasks load as target/<name>
path = "../target-repo"           # local checkout path visible to the plane
repo_url = "https://github.com/o/r.git"   # optional; workspace clone URL, defaults to path
ref = "main"                      # default: master; recorded on every run
agent = "reviewer"                # plane-owned binding
substrate = "sprites"             # plane-owned substrate

[workload_repo.workspace]         # plane-owned workspace authority
host = "bb-target"
checkpoint = "v3"

[workload_repo.budget_caps]       # ceilings/defaults granted to repo tasks
timeout_minutes = 30
max_runs_per_day = 10
max_cost_per_run_usd = 2.0
turn_cap = 50
```

Repo-owned `task.toml` owns the card, triggers, adapter commands, verdict
marker, and optional budget requests that stay within the plane-granted
caps. Agent binding, substrate, workspace host/repos/checkpoint, and budget
ceilings remain plane-owned. `bb check` fails when a repo task names an
unknown or ungranted agent, requests `substrate = "local"` (unless the
plane granted local on a dev plane), exceeds a budget cap, or attempts to
declare workspace authority. Removing a `[[workload_repo]]` entry removes
its namespaced tasks and trigger routes on the next config load; dispatch
workers reload config for each run, HTTP ingress reloads per request, and
cron refreshes on a bounded poll.

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

[workspace]                       # materialized by the substrate adapter
host = "bb-demo"                  # substrate resource identity / host lease key
repos = [{ url = "https://github.com/o/r.git", ref = "master" }]
checkpoint = "v3"                 # optional snapshot; ignored by adapters without snapshots

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

## Substrate contract

Dispatch supplies a declarative `WorkspacePlan`: task workspace name,
declared repos, card, EVENT/REPORT payloads, pre/post commands, probe
marker, optional checkpoint, resolved secrets, and hermeticity. The
adapter owns every environment-specific choice behind that plan:

- Map the workspace name to the adapter's own workspace location or
  resource. Dispatch never constructs host paths.
- Restore the checkpoint before prepare when the adapter supports
  snapshots; adapters without snapshots ignore it.
- Materialize repos at declared refs plus `LANE_CARD.md`, `EVENT.json`,
  and `REPORT.json`; run `pre_command` without starting the agent.
- Execute the harness in the prepared workspace with a wall-clock kill
  and probe marker, write artifacts, then release adapter resources.

`local` keeps a workspace under the attempt directory for dev/test planes.
`sprites` maps the workspace name onto its remote overlay and handles the
sprite CLI transport entirely inside `src/substrate/sprites.rs`.

## Environment Contract

The v1 environment contract is the task `WorkspacePlan`: declared repos,
optional sprite checkpoint, task `pre_command`, card, event, report,
declared secrets, and task `post_command`. The plane does not honor
`devcontainer.json`.

That omission is deliberate. The Dev Container spec describes a container
environment, including image/build settings, Feature installation, and
lifecycle commands. Features are self-contained install units with
metadata plus an `install.sh`, and lifecycle commands such as
`postCreateCommand` run after the container has been created. A Fly Sprite
workspace is not a container build; it is a restored machine plus repo
sync. Running only `postCreateCommand` would mimic one lifecycle hook
while ignoring image, Feature, mount, user, and container semantics, which
is worse than an honest unsupported boundary.

For repo-specific setup today, use a task `pre_command` for per-run
workspace preparation or a sprite checkpoint for machine-level tools.
Reconsider devcontainer support only behind a substrate that can delegate
to a real devcontainer implementation or container runtime without moving
Feature/package judgment into the Rust spine.

## Durable reflex deployment

The operator plane runs as the Fly app `bitterblossom-plane`, not as a
laptop process. The checked-in [plane/plane.toml](../plane/plane.toml) stays
loopback-local for safe ad hoc use; Fly sets `BB_INGRESS_BIND=0.0.0.0:8080`
so its public URL can reach `bb serve`. Set `BB_API_TOKEN` before binding
wider: the read API and HTML operator view are token-gated, and `bb serve`
refuses non-loopback binds without it. `/health` stays unauthenticated for
liveness and exposes only coarse queue counts; webhook ingress is
authenticated by each trigger's HMAC secret.

Deployment contract:

- Host: Fly app `bitterblossom-plane` in org `misty-step`, one always-on
  `dfw` machine, command `bb --config plane serve`.
- State: encrypted Fly volume `bb_plane_data` mounted at `/app/plane/.bb`, so
  the checked-in relative `db_path = ".bb/plane.db"` is volume-backed in
  production.
- Image: [Dockerfile](../Dockerfile) builds the Rust `bb` binary and installs
  the pinned Linux Sprite CLI; [fly.toml](../fly.toml) sets `BB_SPRITE_BIN`.
- Runtime secrets live on Fly, never in git: `BB_API_TOKEN`,
  `BB_HOOK_REVIEW`, `OPENROUTER_API_KEY`, `GH_TOKEN`, and `SPRITE_TOKEN`.
- GitHub `pull_request` webhooks for the reviewed repo subset point at
  `https://bitterblossom-plane.fly.dev/hooks/review`; the current subset is
  `misty-step/bitterblossom`, enforced again by the task filter.
- Health and recovery checks after a host restart are: unauthenticated
  `GET /health`, `GET /api/tasks` with `Authorization: Bearer $BB_API_TOKEN`,
  `flyctl status --app bitterblossom-plane`, `flyctl volumes list --app
  bitterblossom-plane`, and `bb --config plane recover` inside the Fly
  machine.

The GitHub webhook is deliberately a per-repo hook for v1, not a GitHub
App. It exercises the same HMAC/dedupe/filter path as a future App
delivery and keeps the reviewed subset operator-editable in GitHub's hook
configuration while the plane remains workload-agnostic.

## Observability

The ledger is the system of record; everything reads from it:

- `GET /` — server-rendered HTML operator view (tasks, budgets, parked
  state, recent runs; auto-refreshes).
- `GET /api/runs[?task=&state=]`, `GET /api/runs/<id>` (run + attempts +
  events), `GET /api/dlq`, `GET /api/tasks`, `GET /api/submissions`
  (submissions + verdicts + rejection reasons) — the agent-facing read
  API, same shapes as the `--json` CLI.
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
