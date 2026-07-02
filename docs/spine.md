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
agents/<name>.toml          # versioned launch contract: role, skills, harness, model, flags
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
tool_action_cap = 100
output_bytes_cap = 50000
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

[backup]                          # optional; projected by `bb status --json`
enabled = true
provider = "litestream"
replica_env = "LITESTREAM_REPLICA_URL"     # env name only, never the value
last_success_path = ".bb/backup-last-success"
rpo_seconds = 300
rto_seconds = 1800

[budget]
max_cost_per_day_usd = 25.0       # global daily ceiling, enforced pre-dispatch
```

## agents/<name>.toml

```toml
version = 1                       # bump on any change; recorded on every attempt
role = "reviewer"                 # operator-facing role: builder, critic, verifier, gardener...
harness = "pi"                    # claude | codex | pi | omp | command
model = "moonshotai/kimi-k2.6"
provider = "openrouter"           # pi/omp only; defaults to "openrouter"
auth = "api"                      # api | subscription (defaults by harness)
bin = "pi"                        # optional: override the harness binary path
args = []                         # optional: extra CLI args appended verbatim
secrets = ["OPENROUTER_API_KEY"]  # env names resolved per-exec, never persisted

skills = [                         # curated role contract; v1 records/exposes,
  "harness-kit/code-review#coordinator", # but does not project skills at runtime
]

[policy]                           # optional per-agent governance boundary
provider_key_name = "openrouter-reviewer"
provider_spend_cap_usd = 10.0      # mapped to OpenRouter key `limit`
model_allowlist = ["moonshotai/kimi-k2.6"]
trigger_bindings = ["manual", "webhook"]
iteration_cap = 24                 # composed with task turn_cap; strictest wins
turn_cap = 40                      # enforced via harness flags or stream monitor
tool_action_cap = 80               # enforced for JSONL tool-call harnesses
output_bytes_cap = 30000           # enforced by streaming stdout/stderr monitor
wall_clock_minutes = 30            # composed with task timeout; strictest wins
side_effect_policy = "kill"        # kill | quarantine | log on in-flight/policy overrun
```

Unsupported loop caps fail before command construction. Generic `command`
agents may use wall-clock and output-byte caps; turn, iteration, and tool-action
caps belong to harnesses with native flags or streamed machine-readable progress.

Swapping a task's agent is a one-line edit to `task.toml`; the ledger
records which agent name + version produced every attempt.

`role` and `skills` are launch-contract metadata: visible in
`bb check --json`, `/api/tasks`, and `bb task list --json` so operators and
agents can tell what kind of worker is bound before dispatch. They do not
change execution in v1. A runtime skill-projection system belongs behind a
separate shaped contract so dispatch stays a thin card + harness runner, not
a semantic workflow engine.

When an OpenRouter API-auth agent declares `policy.provider_key_name` and
`policy.provider_spend_cap_usd`, `bb keys mint <agent>` creates a child API key
through the OpenRouter management API using `OPENROUTER_MANAGEMENT_KEY`, stores
the one-time plaintext child key under the plane's `.bb/` state, and sets the
provider-side `limit` to the policy cap. Dispatch then injects that child key as
the agent's declared `OPENROUTER_API_KEY`; it does not fall back to a shared
key for policy-bound agents.

### Model & auth policy (enforced at load — `bb check` fails, not dispatch)

Two auth classes, two work classes:

- **`subscription`** (claude/codex default): the agent runs *as* the
  operator on OAuth subscription auth. API keys are forbidden —
  `ANTHROPIC_API_KEY`/`OPENAI_API_KEY` as agent secrets fail the load, as
  does `auth = "api"` on those harnesses. Subscription agents bind only
  to manual-only tasks (**dispatch** work).
- **`api`** (pi/omp default): cheap open-weight models via OpenRouter. The
  only class allowed on webhook/cron triggers (**reflex** work). Execs
  are hermetic: scrubbed environment, workspace-local HOME, declared
  secrets only — nothing of the operator's identity crosses the exec
  boundary.

## tasks/<name>/task.toml

```toml
agent = "reviewer"                # agents/reviewer.toml
substrate = "sprites"             # remote-only; "local" needs plane dev = true
required_artifacts = ["REPORT.json"] # optional: zero-exit success requires these

[workspace]                       # materialized by the substrate adapter
host = "bb-demo"                  # substrate resource identity / host lease key
repos = [{ url = "https://github.com/o/r.git", ref = "master" }]
checkpoint = "v3"                 # optional snapshot; ignored by adapters without snapshots

[budget]
timeout_minutes = 30              # enforced: wall-clock cancel
max_runs_per_day = 10             # enforced pre-dispatch
max_cost_per_run_usd = 2.0        # enforced in-flight when usage streams
turn_cap = 50                     # composed with agent policy caps; strictest wins
tool_action_cap = 100             # enforced for JSONL tool-call harnesses
output_bytes_cap = 50000          # enforced by streaming stdout/stderr monitor

[[trigger]]
kind = "manual"                   # `bb run <task>`; the degenerate trigger

[[trigger]]
kind = "cron"
schedule = "0 */6 * * *"          # dedupe key = the scheduled timestamp

[[trigger]]
kind = "webhook"
route = "demo"                    # POST /hooks/demo
secret_env = "BB_HOOK_DEMO"       # HMAC-SHA256 secret env var
dedupe_key = "header:X-GitHub-Delivery"   # or "json:<pointer>[|json:<pointer>]"

# Optional trigger-time expansion. This keeps recurring lifecycle orchestration
# data-owned: the webhook delivery opens or reuses a submission and enqueues
# the gate-required verdict tasks with canonical `storm:<submission>:<kind>`
# idempotency keys.
[trigger.action]
kind = "submission_storm"
change = "json:/pull_request/html_url"
rev = "json:/pull_request/head/sha"
repo = "json:/repository/full_name"
version = "json:/pull_request/updated_at"   # rejects late stale heads

# Containment filters (ANDed; fail-closed on missing pointers). An
# authenticated delivery failing any filter is acknowledged with 200
# but never becomes a run — scope lives in config, not card prose.
[[trigger.filter]]
pointer = "/repository/full_name"
any_of = ["misty-step/bitterblossom"]
[[trigger.filter]]
pointer = "/pull_request/draft"
equals = false

pre_command = ""                  # optional adapter commands run in the
post_command = ""                 # workspace before/after the agent
```

`required_artifacts` is a completion contract, not a prompt hint. Entries are
non-empty paths relative to the attempt artifact directory; absolute paths,
`.` and `..` are rejected at config load. Current substrates release
`REPORT.json`, so other required paths are rejected until artifact transport is
generalized. After a zero-exit harness run and substrate release, every listed
path must exist in the attempt artifact directory or dispatch records the
attempt as `failure` while preserving stdout/stderr/result artifacts for
inspection. Use it for report-producing workloads such as builders,
diagnosers, gardeners, and model evaluators.

### Manual builder dispatch

The checked-in `build` task is the first authoring lane. It binds
`bb-builder-rust@v1` to a manual-only sprites task and commissions exactly
one shaped slice from a backlog item or context packet. The builder may
create commits and push a branch, but it never merges, parks tasks, edits
secrets, or runs from webhook/cron triggers. The existing submission loop
remains the acceptance path: submit the produced branch, run the storm, then
gate.

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

The operator plane runs as a hosted app, not as a laptop process. Product
images contain the `bb` binary and substrate tools only; the production
`plane.toml`, `agents/`, `tasks/`, cards, budgets, org allowlists, and ledger
state are instance data supplied at runtime. Fly sets
`BB_INGRESS_BIND=0.0.0.0:8080` so its public URL can reach `bb serve`; set
`BB_API_TOKEN` before binding wider because the read API and HTML operator shell
are token-gated, and `bb serve` refuses non-loopback binds without it.
`/health` stays unauthenticated for liveness and exposes only coarse queue
counts; webhook ingress is authenticated by each trigger's HMAC secret.

Deployment contract:

- Host: one always-on Fly machine, command `bb serve`, with
  `BB_PLANE_DIR=/app/plane`.
- Config and state: encrypted Fly volume `bb_plane_data` mounted at
  `/app/plane`. The volume root contains `plane.toml`, `agents/`, `tasks/`, and
  `.bb/plane.db`, so ordinary card, budget, and allowlist changes are runtime
  config changes rather than image rebuilds.
- Backup readiness: `[backup]` in `plane.toml` declares the provider, replica
  secret env name, RPO/RTO, and heartbeat file. `bb status --json` reports
  `backup.status` from that heartbeat without reading replica secrets; a stale
  or missing heartbeat means the ledger is not protected enough for unattended
  growth. The production image starts through `bb-litestream-entrypoint`, which
  fails closed when `BB_LITESTREAM_REQUIRED=1` and the replica secret env is
  missing, starts `litestream replicate -config`, waits for the first
  `litestream sync -wait`, and writes the heartbeat only after sync confirms the
  volume-backed ledger has replicated.
- Schema rollback: `bb` stamps SQLite `PRAGMA user_version` as
  `ledger.schema_version` in `bb status --json`/`bb check --json` and refuses to
  open a ledger newer than the binary supports. Rollbacks across a schema bump
  must roll forward or restore a compatible backup; never force `user_version`
  downward.
- Image: [Dockerfile](../Dockerfile) builds the Rust `bb` binary and installs
  the pinned Linux Sprite CLI plus pinned Litestream; it must not `COPY plane`.
  [fly.toml](../fly.toml) sets `BB_PLANE_DIR`, `BB_SPRITE_BIN`, and the
  Litestream env-name contract without storing the replica URL.
- Runtime secrets live on Fly, never in git: `BB_API_TOKEN`,
  `BB_HOOK_REVIEW`, `BB_HOOK_CI_DIAGNOSE`, `OPENROUTER_API_KEY`, `GH_TOKEN`,
  `SPRITE_TOKEN`, and `LITESTREAM_REPLICA_URL`.
- GitHub `pull_request` webhooks for the reviewed repo subset point at
  `https://bitterblossom-plane.fly.dev/hooks/review`; the current subset is
  `misty-step/bitterblossom`, enforced again by the task filter. In-scope
  `opened`, `ready_for_review`, and `synchronize` deliveries create the review
  run and expand into the submission storm automatically: one submission keyed
  by the PR URL plus one run for every `[gate].required` verdict member,
  deduped by PR URL plus head SHA so redeliveries repair missing member rows
  without collapsing distinct PRs that share a commit, and a redelivery of a
  head whose submission has already settled is an idempotent no-op. Large PRs
  are not filtered by additions count: the `review` task carries no per-run cost
  cap (a breached cap parks the whole task, which is why it was dropped), so
  spend is bounded by the 30-minute per-run timeout, `max_runs_per_day`, and the
  plane's enforced `max_cost_per_day_usd` daily ceiling — not by a per-run
  dollar cap.
- GitHub `check_suite` webhooks for failed GitHub Actions suites point at
  `https://bitterblossom-plane.fly.dev/hooks/ci-diagnose`; the first slice is
  report-only and may recommend a builder command, but never creates one.
- Health and recovery checks after a host restart are: unauthenticated
  `GET /health`, `GET /api/tasks` with `Authorization: Bearer $BB_API_TOKEN`,
  `flyctl status --app bitterblossom-plane`, `flyctl volumes list --app
  bitterblossom-plane`, and `BB_PLANE_DIR=/app/plane bb recover` inside the Fly
  machine.
- Production operations live in [`docs/operations/`](operations/). The
  maintained smoke and restore drill is
  `./scripts/production-ops-drill.sh --local` for CI/local proof and
  `BB_API_TOKEN=... ./scripts/production-ops-drill.sh --remote` for the Fly
  plane.

The GitHub webhook is deliberately a per-repo hook for v1, not a GitHub
App. It exercises the same HMAC/dedupe/filter path as a future App
delivery and keeps the reviewed subset operator-editable in GitHub's hook
configuration while the plane remains workload-agnostic.

## Observability

The ledger is the system of record; everything reads from it:

- `GET /` — token-gated HTML operator shell linking the read API below.
- `GET /api/runs[?task=&state=]`, `GET /api/runs/<id>` (run + attempts +
  events), `GET /api/dlq`, `GET /api/notify`, `GET /api/tasks`,
  `GET /api/submissions`
  (submissions + top-level `id/change_key/rev/round/state` summary fields +
  nested submission details, verdicts, and rejection reasons) — the
  agent-facing read API, same shapes as the `--json` CLI.
- The required v1 read-surface fields are pinned in
  `tests/fixtures/contracts/bb.agent_read_surfaces.v1.schema.json` and
  validated against live CLI output plus HTTP API mirrors by
  `tests/agent_contract_fixtures.rs`. Additive fields are allowed; removing
  or renaming a required path needs a new major schema.
- Auth: set `BB_API_TOKEN` on the plane and send
  `Authorization: Bearer <token>`. Query-string tokens are rejected so
  credentials do not leak through URLs, logs, or browser history. Unset =
  open, acceptable only on the loopback default bind.

Cost attribution rides OpenRouter's per-response usage accounting
(`usage.cost` arrives with pi/omp responses — no extra calls), parsed
per attempt into the ledger. Decision 2026-06-10: no OTel/Langfuse
sidecar for now; `bb runs export` is the versioned telemetry seam for
Daedalus handoff and future `gen_ai.*` adapters. The v1 schema and
compatibility rules live in `docs/run-telemetry-export-v1.md`.

The review workload also supports explicit manual tokenomics probes:
`bb --config <runtime-plane> run review --payload '{"repo":"o/r","pr":N,"measurement":true}'`.
Measurement mode runs the same real PR review path but suppresses the
GitHub comment and leaves the full findings in `result.md`; webhook
reviews post exactly one PR comment and also start the mechanical submission
storm.

The CI-diagnose workload supports manual dogfood:
`bb --config <runtime-plane> run ci-diagnose --payload '{"repo":"o/r","head_sha":"SHA","workflow":"verify"}'`.
It writes a report/fix packet and may recommend a builder command, but does not
edit code, post comments, or trigger follow-up runs.

The model-evaluation loop supports manual candidate comparison. Run at least
three candidate tasks for the same flow and payload, then pass their reports to
`bb --config <runtime-plane> run model-eval --payload '<json>' --json`. First-class
cohorts are listed in [`docs/model-evals/`](model-evals/README.md) and cover
`build`, `review`, `gardener`, `ci-diagnose`, and the submission-storm member
flows.

Candidate variants are manual-only. Review variants force measurement mode;
gardener variants force dry-run; build variants default to dry-run unless the
payload explicitly asks for a live branch; storm variants use eval-only verdict
kinds and do not change gate arithmetic.

The evaluator is `model-eval` on `openai/gpt-5.5` through OpenRouter API auth.
It writes `REPORT.json`; the operator records accepted findings under
[`docs/model-evals/`](model-evals/README.md) as future reference context.
`z-ai/glm-5.2` is in the OpenRouter API catalog as checked on June 16 and
June 18, 2026, so the runnable GLM-family candidate tasks use it. The manual
`build` default now uses OMP/GLM to avoid sprite-side Codex subscription-token
rotation; the other flow defaults stay on their evaluated open-model configs
until a flow-specific model-eval record promotes them. Historical model-eval
records keep their original model ids when the actual run used GLM 5.1.

The model-catalog watcher keeps that reference current without making model
promotion automatic. `./scripts/verify.sh` runs
`scripts/check-model-catalog.sh --catalog tests/fixtures/openrouter-models-current.json`
against a checked-in OpenRouter fixture, so local and CI gates stay
deterministic. Live discovery runs through
`bb --config <runtime-plane> run model-catalog-watch --payload '{"dry_run":true}' --json`;
the task fetches the live catalog, reports fixture/config/docs drift and
same-family candidates in `REPORT.json`, and must not edit runtime agent config.
Changing a default still requires a flow-specific `bb` smoke run plus a
model-eval record.

## The submission loop

Completed agent work is quality-assured and landed by a **verdict storm**
plus a **mechanical gate** — no human reads the code, no PR is the
channel. The spine holds the data mechanics only (submissions, verdicts,
gate arithmetic); what a reviewer looks for lives in cards.

**Submissions.** `bb submit open --change <key> --rev <sha>` creates a
submission: `open → clear | blocked | escalated | abandoned`, at most one
non-terminal submission per change key (CAS-enforced). `bb submit list --json`
returns recent submissions (default 20, `--limit` clamped to 1..=200 like
`/api/submissions`) with top-level `id`, `change_key`, `rev`, `round`, and
`state` fields plus the nested `submission` details, verdict rows, and rejection
reasons, giving cron supervisors and agents a typed way to discover active or
stale gate work without querying SQLite or guessing receipt shapes. Summary
recipe: `bb submit list --json | jq '.[] | {id, change_key, rev, state, round}'`
(same row shape from `/api/submissions`). The change key and rev are opaque
strings (branch + SHA today; jj change IDs later, zero spine change). Round
numbering is plane-owned: reopening after `blocked` increments the round and
snapshots the prior gate report — the driver cannot soften or omit prior
findings; verdict tasks receive the canonical report as `REPORT.json` next to
`EVENT.json`.

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

`simplification` is the read-only refactor lens. Refactor diffs come from
`build` or the operator and then re-enter the normal storm; there is no
standalone mutating refactor workload in v1. See
[`refactor-lens.md`](refactor-lens.md).

**The gate.** `plane.toml` declares policy; `bb gate --change <key>`
evaluates pure data:

```toml
[gate]
required = ["verify", "correctness", "security", "simplification", "product"]
quorum = 5                       # optional; defaults to required.len()
arm_timeout_seconds = 3600       # pending/not-started members cannot hang forever
max_rounds = 3                    # rounds 1..=max_rounds may run
arbiter = "arbiter"               # verdict kind that settles disputes
```

- A required kind without a terminal run → `pending` (per-kind states
  listed) until it breaches `arm_timeout_seconds`. A timed-out or terminally
  failed member is unavailable: if the configured quorum cannot still be met,
  the submission **escalates** through the notification outbox instead of
  staying pending forever. Unavailable members include `safe_next_command` and
  `safe_next_reason` in JSON. With no explicit `quorum`, all required members
  are still required, preserving the original all-hands gate. With an explicit
  lower quorum, a timed-out extra arm may allow `clear`, but the timeout still
  emits a durable `submission_arm_timed_out` notification.
- **Only `blocking`-severity findings block — every round.** Fresh
  blockers are never demoted by recency; termination rests solely on the
  round cap. `serious`/`minor` never block (anti-needling is mechanical).
- A rejected fingerprint (`bb submit reject`) cannot block again — but
  rejecting a `blocking` finding only takes effect once an **arbiter**
  verdict independently sustains the rejection. Rejections and reasons
  appear verbatim in every subsequent report.
- `blocked` at `round == max_rounds` → `escalated` (one notify).

**The driver (convention, not spine).** Manual dispatch can still run the
loop explicitly: push the branch → `bb submit open` → fire required storm
members as parallel `bb run <kind> --idempotency-key
storm:<submission>:<kind> --payload '{"submission":"<id>", ...}'` → `bb gate`
(safe to call any time). For PR lifecycle work, the `review` webhook performs
that opening fan-out automatically. On `clear`: file advisories to backlog.d,
squash-land (`clear` is terminal); on `blocked`: fix, push, and the next
`synchronize` delivery opens the next review submission; on `escalated`: stop
— the operator is already notified. Judgment (what to fix, what to reject)
stays with the agent; arithmetic lives in `bb gate`.

## Run lifecycle

A durable run row exists in SQLite **before any trigger gets its ack**.
States: `pending → running → success | failure | awaiting_recovery`, plus
`blocked_budget` (ingress on a parked task or over a budget limit; recorded,
never dispatched) and `retired` (a `blocked_budget` run an operator closed).
A `blocked_budget` run is recovered run-by-run: `bb runs release <id>` re-queues
one run and unparks its task (refused when releasing would just re-block it — a
ceiling with no park, or a daily run/cost limit still over), `bb runs retire
<id> --reason …` closes one as `retired` (terminal, row kept), and `bb task
unpark <task>` still releases the whole parked queue at once. Each dispatch
attempt checkpoints its phase —
`acquired → prepared → executing → collecting → finalizing → released` —
because agent runs have external side effects and "re-run it" is not a
recovery semantic:

- Failures **before** `executing` retry mechanically (2 retries), then
  dead-letter with full payload + attempt history.
- Failures at or after `executing` go straight to `failure` or
  `awaiting_recovery`; replay is an explicit operator act.
- On boot, inherited `running` runs are classified by probing the host and
  reading attempt artifacts — never blindly orphaned.
- Probe results are explicit and machine-readable in `recover --json`:
  `probe_state` is `alive`, `dead`, or `unknown`; `probe_reason` explains
  unknowns; `lease_disposition` is `retained` or `released`; and
  `operator_action` names the safe next step. The legacy human `probe` string
  remains for compatibility.
- State machine: local probes use the attempt-scoped `harness.pid`; sprite
  probes use the attempt marker `/tmp/<attempt-id>.pid`. `alive` keeps the run
  `running` and retains the lease. `dead` moves to `awaiting_recovery` and
  releases the lease. `unknown` moves to `awaiting_recovery` while retaining
  the lease because the plane cannot prove the agent process is gone. Missing
  or malformed pidfiles, probe command failures, unknown substrates, and
  unknown tasks are unknown.
- `bb status --json` is the stale recovery visibility surface: after one hour,
  an `awaiting_recovery` run's safe action changes from
  `resolve_after_side_effect_inspection` to `escalate_stale_recovery`, with
  `age_seconds` and `stale_after_seconds` included. The plane does not resolve
  or replay side-effecting work automatically.
- The full run-state, attempt-phase, and notification freshness contract is
  documented in [`freshness-contracts.md`](freshness-contracts.md) and emitted
  by `bb status --json` under `freshness_contracts`.
- `bb dlq replay <id> [--json]` mints a **new** run linked via
  `parent_run_id`; JSON mode returns the replayed run + attempts + events.
- `bb dlq ack <id> --reason TEXT [--json]` acknowledges a pre-execute dead
  letter as superseded without replaying it, recording reason + timestamp.
  Acknowledgement and replay are mutually exclusive: an acknowledged DLQ
  cannot be replayed, and a replayed DLQ cannot be acknowledged. Replay
  history (`replayed_run_id`) is immutable. `bb dlq list --json` reports each
  row's `status` (`open`, `replayed`, or `acknowledged`) with acknowledgement
  reason and timestamp; `bb status --json` counts only `open` rows as
  unresolved operator work.

Host mutual exclusion is a durable lease keyed by substrate resource
identity (the sprite/host), not by task: two tasks sharing a host never
overlap. Per-task FIFO ordering is layered above that lease.

## Operator CLI

All read commands take `--json` and emit stable shapes (agents are users).
`bb run --json` prints only the final run bundle; human-mode `bb run` prints an
early run id plus periodic heartbeat lines on stderr while dispatch is in
progress.

```
bb run <task> [--idempotency-key K] [--payload JSON | --payload-file PATH] [--json] # manual trigger; payload validated as JSON before ingest
bb status [--json]                                # task/run/queue/DLQ health
bb runs list [--task T] [--state S] [--json]
bb runs show <run-id> [--json]                    # run + attempts + events
bb runs release <id> [--reason TEXT]              # re-queue ONE blocked_budget run
bb runs retire <id> --reason TEXT                 # blocked_budget -> retired (terminal)
bb runs export                                    # bb.run_telemetry.v1 JSONL
bb artifacts list <run-id> [--json]                # top-level artifact files across a run's attempts
bb artifacts read <run-id> <path> [--json]         # safe text/JSON read, including known nested relative paths; binary/oversized/unsafe paths refused
bb dlq list [--json]
bb dlq replay <id> [--json]
bb dlq ack <id> --reason TEXT [--json]            # close a superseded pre-execute DLQ
bb notify list [--limit N] [--json]               # outbound notification outbox
bb notify retry [--limit N] [--json]              # retry pending/failed webhook deliveries
bb notify ack <id> --reason TEXT [--json]         # close a handled notification row
bb keys mint <agent> | --all [--json]             # mint scoped OpenRouter child keys from agent policy caps
bb keys rotate <agent> [--json]                   # replace one stored child key, revoke the old key
bb keys revoke <agent> [--json]                   # revoke one stored child key and clear local key material
bb keys list [--remote] [--include-disabled] [--json] # local metadata or OpenRouter management list
bb preflight <task> | --storm [--json]            # missing secrets, local command bins, subscription auth readiness; pre-dispatch
bb task list [--json]                               # agent-facing task inventory
bb task park|unpark <task>
bb submit open --change K --rev SHA [--context TEXT]
scripts/bb-submit-storm --config <plane> --payload-file storm.json [--bb "target/debug/bb"] [--json] # checked-in operator recipe: validate payload, storm preflight, open, run members via --payload-file, return receipt
bb submit reject --change K --fingerprint FP --reason TEXT
bb submit abandon --change K
bb gate --change K | --submission ID [--json]     # also GET /api/gate?change=K
bb serve                                          # webhook + cron + queue
bb mcp serve                                      # read-only MCP stdio server (bb_status, bb_check, bb_tasks, bb_runs_list, bb_runs_show, bb_artifacts_list, bb_artifact_read, bb_dlq_list, bb_preflight, bb_gate); JSON-RPC over stdin/stdout
```

Cost and tokens are parsed from harness output per attempt; unparseable
output is a `failure` with raw output preserved on the attempt row — never
a silent zero-cost success.

`bb preflight` is a read-only pre-dispatch check, not a gate: it reports
missing declared secrets, missing policy-bound provider keys, and unspawnable
`command`-harness binaries (the `/bin/true`-missing and missing-`GH_TOKEN`
storm classes) for one task or the submission-storm member set, before dispatch
creates run rows. Secret and provider-key checks apply on every substrate;
binary checks apply only to `substrate = "local"`, the only substrate whose
binaries `bb` can inspect.

For manual-only Codex/Claude subscription-auth tasks, preflight also reports a
classified readiness finding before authoring begins. Operators can set
`BB_PREFLIGHT_SUBSCRIPTION_AUTH_PROBE_CODEX`,
`BB_PREFLIGHT_SUBSCRIPTION_AUTH_PROBE_CLAUDE`, or the generic
`BB_PREFLIGHT_SUBSCRIPTION_AUTH_PROBE` to a read-only probe executable. The
probe receives `BB_PREFLIGHT_TASK`, `BB_PREFLIGHT_HOST`,
`BB_PREFLIGHT_SUBSTRATE`, `BB_PREFLIGHT_HARNESS`, `BB_PREFLIGHT_BIN`, and
`BB_PREFLIGHT_MODEL`; zero means ready, non-zero becomes a
`subscription_auth_unready` finding whose JSON names the task, host, substrate,
harness, binary, model, classification (`readiness`), detail, and remediation.
Without a configured probe, subscription-auth tasks report
`subscription_auth_unverified` instead of silently creating an implementation
run to discover expired OAuth state.

## What the plane refuses to know

No workload logic, no judgment. Retry semantics are mechanical; agents own
their own decomposition. If a feature needs a workload-specific branch in
dispatch/queue/substrate, it belongs in the task spec or in harness-kit.
The spine LOC tripwire lives in `scripts/verify.sh`; it is a bloat audit, not a
workload-judgment budget. Design rationale and critique record:
`docs/plans/2026-06-10-031-event-plane-spine.md`, ADR 005.
