# Bitterblossom Vision

Status: canonical north star. Lifespan: long-lived product substrate for Misty
Step first, designed so other agent operators can adopt the same shape without
inheriting our repos, models, or GitHub conventions.

Direction lock, 2026-06-29: Misty Step will route more of its agent operations
through Bitterblossom. The product must support both supervised dispatch and
unsupervised reflex work as first-class modes; the split is an authority and
auth boundary, not a product fork.

Direction lock, 2026-07-01: Bitterblossom is the Weave execution plane. Heavy
agent execution moves here first: backlog chewing, review dispatch, incident
triage, doc sync, CI audit, and ad-hoc repo work. The product must make
execution cheap and observable while keeping judgment in specialist agents and
human/frontier review loops.

Direction lock, 2026-07-07: every agent run the operator starts anywhere —
including interactive, supervised lead sessions on a local machine — should be
visible through the plane: a ledger row, a trace, receipts, and a glass feed.
The plane does not have to *execute* an interactive session to make it
bb-native; it is enough that the session registers and streams its lifecycle so
the operator sees all fleet activity in one place. Registration is observation,
not a new authority the plane holds over the session (see "Registration is
observation, not authority" and the refusal it qualifies). This lock is why an
interactive lead session, a local codex lane, and a webhook reflex all cohere
into one observable fleet without collapsing into one authority model.

Bitterblossom is the event plane for recurring agent workloads. It lets an
operator define a task, bind an agent, attach a trigger, and then watch the work
run durably on isolated infrastructure with cost, budget, queue, trace, and
recovery state visible from the CLI.

The product is the deterministic waist between events and agents. The plane
accepts work, records it before acknowledging the trigger, prepares the declared
workspace, runs the selected harness, captures artifacts, accounts for cost, and
leaves the operator with an auditable next action. It does not decide what the
workload means.

## The premise that makes this Bitterblossom

Three bets make Bitterblossom different from another agent framework or workflow
runner:

1. **Workloads are files, not runtime branches.** A new recurring workflow
   should be a task card, an agent binding, a trigger, budgets, and artifact
   contracts. If the Rust spine needs a workload-specific branch, the boundary
   has failed.
2. **The plane owns mechanics; specialist brains own judgment.** Bitterblossom
   owns ingress, dedupe, queues, leases, recovery, budgets, substrate execution,
   run receipts, and operator truth. Code review, incident diagnosis, docs sync,
   model evaluation, and ad-hoc authoring are external workload brains or
   operator tools. They arrive behind the same plane contract; the plane should
   not know their product identities.
3. **Supervised and unsupervised operations share one spine.** Manual builder
   dispatch, explicit operator commissions, webhook reflexes, cron watchers, and
   future unattended lifecycle tasks all need the same ledger, substrate,
   artifact, budget, and recovery primitives. They differ in authority, auth,
   trigger source, and side-effect rules; they should not become separate
   systems unless live evidence proves the shared spine cannot serve both.

## Who would miss it

The first user is the operator running a portfolio of repos and operations with
recurring agent work that should not depend on a laptop tab staying open: PR
review, CI failure diagnosis, gate-blocked fix packets, scheduled probes, deploy
verification, production incident diagnosis, and eventually docs or maintenance
watchers.

The second user is the operator's ad-hoc agent. It needs a stable CLI and JSON
surface that can answer: what is configured, what is running, what failed, what
cost money, what is safe to replay, and which work item should be touched next.

If Bitterblossom disappeared, the missing piece would not be "an AI reviewer" or
"an autonomous builder." It would be the durable operating shell that turns
specialist agent programs into repeatable, budgeted, inspectable event-driven
work.

In the Weave loop, Bitterblossom is the compute plane. Powder may hold work
state, Cerberus may review, Canary may report incidents, Landmark may write
release intelligence, and Roster may provide skills, but recurring model
calls and remote execution run through BB with per-agent budgets, scoped
authority, artifacts, and recovery evidence.

## What must stay true

- **Run truth precedes external acknowledgement.** Webhooks, cron ticks, and
  manual dispatch all converge on a ledger row before acceptance is claimed.
- **Registration is observation, not authority.** A run that originates outside
  the plane — an interactive lead session, a local codex lane, a herdr pane —
  may register itself (`POST /api/external-runs`) to earn a ledger row and a
  glass feed. The plane records and observes such a run; it does not execute,
  lease, budget, or gain authority over it. Visibility through the plane is the
  product; control over an externally-owned session is explicitly not. This is
  the seam that lets "every agent run is bb-visible" grow without every run
  becoming a bb-*dispatched* run.
- **Unattended work fails visibly.** Every non-terminal run and submission needs
  a freshness contract, durable escalation path, and operator-visible next
  action. A state that can go stale without notification is a product bug, not
  an ops inconvenience.
- **Every workload runs from a terminal.** Webhook and cron are triggers, not
  privileged execution paths. A human or agent must be able to replay the same
  workflow deliberately with `bb run`.
- **Reflex and dispatch both stay first-class.** Reflex work is trigger-fired,
  hermetic, API-auth, cheap, and bounded. Dispatch work is deliberate,
  operator- or agent-initiated, and may use subscription-auth builders with
  explicit operator authority. Shared mechanics are good; blurred authority is
  not.
- **Side effects are not replayed casually.** Only pre-execute failures retry
  mechanically. Anything at or after execution is operator-resolved with ledger
  evidence because agents may have commented, pushed, opened tickets, or touched
  external systems.
- **Substrate is a deep module, not the product identity.** Fly/Sprites is the
  current proving adapter and local execution is the dev/test adapter. Any
  future substrate must hide host paths, transport quirks, checkpoints, process
  control, and artifact release behind the same workspace plan before it earns a
  place in the plane.
- **Secrets never become payload.** Credentials travel through declared secret
  names and stdin plumbing, never card prose, argv, logs, persisted DB fields, or
  ad-hoc JSON payloads.
- **BYOK and per-agent governance are default.** Released deployments should let
  operators bring their own model keys. Inside a plane, agent definitions carry
  scoped credentials, spend caps, and loop limits so one workload cannot consume
  the whole budget or authority surface.
- **Observation is part of the product.** Agents and humans should read state
  through `bb ... --json` and API mirrors, not log spelunking or SSH guesses. A
  dashboard may help humans drill down, but the CLI/API is the source of truth.
- **Agent-first means more than shelling out.** The product surface should be one
  core projected through CLI, HTTP/API, MCP, SDK, skill, and a thin human UI. MCP
  tools are designed around agent intent and risk boundaries, not auto-wrapped
  REST endpoints.
- **Product and instance stay separate.** This repo should be public-able at any
  moment. Misty Step task cards, org allowlists, sprite hosts, budgets, and live
  runtime data are instance config, not product code baked into the image.
- **Telemetry leaves the plane.** Runs export enough cost, model, task, artifact,
  and receipt data for external labs and operator tooling to improve agents
  without turning Bitterblossom into the evaluation system.

## What this repo refuses

- **No semantic workflow engine in Rust.** The spine routes, leases, records,
  retries, and exposes state. It does not encode reviewer policy, incident
  triage strategy, product judgment, refactor taste, or model-promotion logic.
- **No resident persona fleet.** Weaver/Thorn/Fern/Tansy and the old autonomous
  factory are prior art. The product is not a cast of named agents living on
  machines; it is the file-defined plane that can run whatever agent contract is
  current.
- **No built-in workload brain.** Code review, incident diagnosis, docs sync, and
  model evaluation are deployable workloads, not features of the plane. A review
  runner deployed here is just a task and agent binding with artifacts and
  receipts; it can be swapped without changing the spine.
- **No GitHub-only core.** GitHub is a valuable first trigger and projection
  surface, not the architecture. Tasks and run artifacts must stay source- and
  host-neutral enough for other event sources and destinations.
- **No dashboard-first operations.** The operator should get notified and then
  inspect. A pane of glass that requires constant watching is a failure of the
  event-plane premise.
- **No hidden authority escalation.** Reflex agents should report, recommend, or
  post within an explicit task contract. Merging, unpark decisions, production
  mutations, and broad rollout remain deliberate operator authority until a
  future vision revision says otherwise. Registering an externally-owned run
  (see "Registration is observation, not authority") is not an exception and not
  a loosening: it grants the plane visibility, never control, so an interactive
  lead session becomes fully bb-observable without the plane acquiring any
  standing always-on presence or new escalation power over it.
- **No substrate lock-in disguised as architecture.** Sprites is valuable
  because it currently fits the remote-workspace problem, not because
  Bitterblossom is a Fly product. Cloudflare, E2B, Modal, Daytona, or another
  substrate may win if they prove better on the same workload contract.

## Strategic bets

1. A small deterministic plane around excellent specialist agents will age
   better than a large agent product that owns every workflow's judgment.
2. Files as the product surface make recurring workflows reviewable, diffable,
   portable, and cheap to change.
3. Durable cost and recovery evidence is more valuable than optimistic
   automation. An unattended agent that fails visibly is useful; one that fails
   silently is worse than no agent.
4. The best first unsupervised proof is not a bigger generic factory. It is a
   thin external-specialist reflex: event in, specialist artifact out, ledger and
   receipts visible, posting enabled only after dry-run evidence earns it.
5. The same plane should support both supervised dispatch and unsupervised
   reflex work because the useful primitive is not autonomy level; it is durable,
   budgeted, inspectable execution.
6. Heavy execution belongs on the plane, not the operator laptop. Frontier tokens
   are reserved for planning and review; cheaper API-auth models and Sprites do
   the bounded execution work whenever they are good enough.
7. The next substrate decision should be empirical. Compare Fly/Sprites,
   Cloudflare Sandbox/Agents, E2B, Modal, Daytona, and durable workflow systems
   against a real coding-harness workload: prepare a repo workspace, stream the
   harness, persist or recover state, capture artifacts, enforce budget, and
   replay from ledger evidence.

## What excellent looks like

- **Near term:** `master` has a boring local gate; `bb status --json` is the
  operator truth surface; `bb run build` can chew real backlog items on Sprites;
  stale executing work and stuck gate arms escalate through a durable
  notification outbox; the ledger is backed up and restore-drilled; and the repo
  can be made public because product code no longer contains Misty Step instance
  config.
- **Medium term:** recurring lifecycle reflexes cover PR review, CI diagnosis,
  Canary triage, doc sync, gate-blocked fix packets, and deploy or production
  verification without adding workflow branches to `src/`. Manual dispatch lanes
  and unattended reflex lanes are both used in real Misty Step operations, each
  with task files, manual payloads, live drills, artifact contracts, cost
  history, per-agent governance, and explicit rollback or operator-resolution
  paths.
- **Long term:** Bitterblossom is the operator's durable Mode B substrate. New
  specialist agents arrive as launch contracts and task files; external labs and
  operator tooling improve their brains from exported run evidence; the plane
  remains small enough that its safety and recovery behavior can be understood in
  one sitting.

## Where it sits

Ad-hoc operator tools are Mode A: interactive sessions on local machines.
Bitterblossom is Mode B: the event plane that turns external workload brains
into recurring, isolated, budgeted, observable workloads. Adjacent systems can
provide agents, consume telemetry, or receive artifacts, but they are
consumers and providers rather than dependencies.

`project.md` records the 2026-06-10 v3 direction lock and historical pivot into
this shape. `docs/spine.md` is the operator contract for the current `bb`
surface. This file decides the product boundary and should change only when live
evidence changes what the plane should be.
