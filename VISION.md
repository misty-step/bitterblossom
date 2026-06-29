# Bitterblossom Vision

Status: canonical north star. Lifespan: long-lived product substrate for Misty
Step first, designed so other agent operators can adopt the same shape without
inheriting our repos, models, or GitHub conventions.

Direction lock, 2026-06-29: Misty Step will route more of its agent operations
through Bitterblossom. The product must support both supervised dispatch and
unsupervised reflex work as first-class modes; the split is an authority and
auth boundary, not a product fork.

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

## What must stay true

- **Run truth precedes external acknowledgement.** Webhooks, cron ticks, and
  manual dispatch all converge on a ledger row before acceptance is claimed.
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
- **Observation is part of the product.** Agents and humans should read state
  through `bb ... --json` and API mirrors, not log spelunking or SSH guesses. A
  dashboard may help humans drill down, but the CLI/API is the source of truth.
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
  future vision revision says otherwise.
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
6. The next substrate decision should be empirical. Compare Fly/Sprites,
   Cloudflare Sandbox/Agents, E2B, Modal, Daytona, and durable workflow systems
   against a real coding-harness workload: prepare a repo workspace, stream the
   harness, persist or recover state, capture artifacts, enforce budget, and
   replay from ledger evidence.

## What excellent looks like

- **Near term:** `master` has a boring local gate; `bb status --json` is the
  operator truth surface; `bb run build` can author one shaped slice without
  discovering remote auth failure too late; the first specialist review reflex
  runs report-only from GitHub PR events and stores request, artifact, cost, and
  receipts in the ledger; the substrate bakeoff has one repeatable workload and
  a scored baseline rather than opinions about infrastructure.
- **Medium term:** recurring lifecycle reflexes cover PR review, CI diagnosis,
  gate-blocked fix packets, and deploy or production verification without adding
  workflow branches to `src/`. Manual dispatch lanes and unattended reflex lanes
  are both used in real Misty Step operations, each with task files, manual
  payloads, live drills, artifact contracts, cost history, and explicit rollback
  or operator-resolution paths.
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
