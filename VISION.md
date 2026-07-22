# Bitterblossom Vision

Status: canonical product north star. Direction locked with the operator on
2026-07-11 after a workflow and dashboard design interrogation.

Bitterblossom is the control plane for unattended, trigger-driven agent
workflows. An operator defines a workflow, attaches a trigger, commissions
Roster agents with natural-language goals, and can then see exactly what is
configured, what is active, what happened, what it cost, what authority it
used, and whether the goal was verified.

The current Rust runtime is the migration source, not the final product model:
it still loads file-defined tasks and agents. The target described here makes a
workflow the primary object and stores active, revisioned configuration in the
plane database. Declarative files remain portable import, export, and optional
GitOps surfaces.

## The initial boundary

Bitterblossom starts strictly with unsupervised work initiated by triggers.
Webhook events, schedules, internal workflow events, tests, and deliberate
replays all enter the same durable run plane.

Interactive lead sessions and supervised local agents are deliberately out of
scope. They may become a future extension after the unattended product is
world-class, but Bitterblossom does not initially register, observe, dispatch,
or explain them. This supersedes the 2026-07-07 external-run registration
direction.

The first user is an operator running recurring agent work across a portfolio:
pull-request review, incident resolution, scheduled stewardship, deploy
verification, and other operations that must not depend on a laptop tab
remaining open.

## Local-primary deployment truth

The shipped single-operator production plane runs on local hardware under the
launchd job `com.misty-step.bb-serve`. Its release binary reads the durable
`plane/` instance and binds only to `127.0.0.1:7093`; production config is
`dev = false` with the explicit `allow_local_substrate = true` grant. SQLite
state lives at `plane/.bb/plane.db` in WAL mode. The separate
`com.misty-step.bb-plane-litestream` sidecar writes
`plane/.bb/backup-last-success` only after a confirmed Litestream sync, and
operator-local credentials never enter Git, argv, or evidence.

The `127.0.0.1:7091` dashboard is a separate dev/demo service. Port
`127.0.0.1:7077` belongs only to explicit isolated fixture configurations; it
is not a production default. Sprites and tailnet remain bounded alternate
substrates for workloads that need stronger isolation, not the local-primary
service. The read-only `scripts/production-ops-drill.sh --primary` reads the
live HTTP health/status/runs/DLQ surfaces and SQLite integrity/count snapshots
without invoking mutating CLI paths; an open DLQ status fails closed.

External dispatch registration, register-through wrappers, and
interactive lead sessions are not part of the current product boundary. They remain historical
implementation material only and must not be presented as the default origin.

## The product model

The configured vocabulary is intentionally small:

| Object | Meaning |
|---|---|
| **Workflow** | The primary object: one complete unattended operation. |
| **Trigger** | A normalized source event that creates a workflow run. |
| **Step** | One agent commissioned with a natural-language goal. |
| **Route** | Where a step result goes next: another step or completion. |
| **Agent** | A reusable, pinned Roster definition referenced by steps. |

The runtime vocabulary is smaller still:

| Object | Meaning |
|---|---|
| **Run** | One accepted instance of a workflow. |
| **Result** | A step's outcome, explanation, receipts, and optional artifacts. |

Everything else is composition. Verification is a step performed by a verifier
agent. A loop is a route back to an earlier step. A handoff is a result with
optional artifacts. A semantic routing decision is an agent step when judgment
is required. Budgets, concurrency, retention, and remediation are policies,
not new graph nouns.

The mental model must remain explainable in one line:

> trigger -> agent with goal -> result -> next agent or done

## Natural language is the program

Most agent harnesses already accept a goal as their native primitive.
Bitterblossom leans into that fact rather than building a second prompt
framework.

During workflow creation, a goal-shaping agent strengthens raw intent with
observable acceptance evidence, boundaries, prohibited actions, relevant
context, assumptions, and ambiguous-completion behavior. The operator sees the
original intent and the enhanced commission and explicitly accepts the latter
before activation.

Structure appears only where deterministic code must decide. A simple terminal
step needs no result schema: successful harness completion means `completed`.
A step with multiple routes receives a tiny completion tool that accepts one
declared outcome, a natural-language summary, and optional artifact or receipt
references. The plane never guesses an outcome by matching keywords in prose.

Agent completion is a claim, not necessarily proof. A workflow may declare no
verification, mechanical checks, an independent verifier step, or both. A
verifier receives the original goal plus the execution evidence and returns an
evidence-backed achievement verdict. Verifier quality is evaluated outside the
run rather than spawning an infinite verifier hierarchy.

## The deterministic waist

Bitterblossom owns mechanics:

- event acceptance, validation, deduplication, and durable acknowledgement;
- revisioned workflow configuration and activation;
- queues, concurrency, leases, substrate preparation, and recovery;
- routes over declared outcomes and observable mechanical facts;
- authorization, budget admission, containment, and credential injection;
- immutable run evidence, costs, notifications, and operator truth.

Agents own judgment:

- interpreting goals and context;
- choosing tools and investigation strategies;
- composing Roster-defined or ephemeral child agents;
- reviewing, diagnosing, repairing, testing, and writing explanations;
- performing declared external side effects within scoped authority;
- emitting the named outcomes on which deterministic routes depend.

A workload-specific semantic branch in the Rust spine is wrong by definition.
So is deterministic keyword routing at a semantic seam. The plane carries
policy and evidence; specialist agents decide what the work means.

## Triggers, results, and loops

There is one Trigger primitive. Its source may be an external connector event,
a clock or schedule event, an event emitted by another workflow, or a manually
synthesized event for test or replay. All sources produce one normalized event
envelope and one durable acceptance path.

Structural filtering such as repository, branch, service, or event type may be
deterministic. Semantic filtering belongs in an agent step.

Artifacts do not fire because a file appeared. A result follows a route or
emits an event and may reference zero or more artifacts. When a handoff
requires an artifact, the plane validates its presence before following the
route.

Workflows may be arbitrarily long, but unattended cycles are bounded. A cycle
declares its semantic exit condition and selects one or more enforceable guards:
rounds, elapsed time, spend, deadline, or an external stop signal. Multiple
guards stop the cycle when the first fires. Exhaustion completes, pauses, or
escalates according to declared policy.

## Roster is the agent source

An agent is a model with a harness operating in an environment. Roster owns the
reusable definition: identity, model policy, harness, instructions, skills, and
maximum capability envelope. Bitterblossom supplies the workflow goal,
environment, context, tools, credentials, and narrower grant.

A plane connects one or more Roster-compatible catalogs. Synchronization
imports immutable agent revisions into the plane database. Workflow activation
pins the exact revision and materialized launch contract so runs do not depend
on a mutable branch or an available catalog service.

Roster must expose catalog loading, validation, and launch-contract
materialization as a stable package interface. Bitterblossom should not copy
Roster identities or require a live source checkout during execution.

An agent created inside the workflow builder becomes a reusable local
Roster-compatible catalog entry. An agent created dynamically by another agent
is ephemeral: it appears only inside the parent run tree, with its complete
launch snapshot, cost, authority, and result retained in history.

Authority narrows monotonically:

> Roster capability ceiling -> workflow grant -> inherited child grant

## Configuration is live, revisioned state

The database is authoritative for currently active workflow configuration.
Dashboard and CLI are projections over the same API. Every edit produces an
immutable revision with an audit trail. Activation selects one revision; runs
remain pinned to the workflow and agent revisions accepted with their trigger.

New activations affect new events only. Existing runs continue unchanged unless
explicitly cancelled. Rollback activates an earlier snapshot as a new revision
rather than rewriting history.

Workflow lifecycle is `draft`, `active`, `paused`, and `archived`. Pausing
suppresses new runs while preserving incoming-event dispositions and allowing
active work to finish. Resume never silently replays suppressed events.
Activated workflows are archived, not deleted, so historical runs retain their
referents.

Declarative workflow documents remain important. They can be imported,
exported, diffed, reviewed, and optionally synchronized through Git. They are
an interchange surface, not a second live source of truth.

## Security, credentials, and access

Security is a first-class workflow concern without turning Bitterblossom into
the fleet's universal security product.

- Roster declares the maximum authority an agent identity can ever receive.
- Bitterblossom grants workflow-scoped capabilities, admits work, injects
  credentials, enforces runtime policy, redacts evidence, and records audit
  receipts.
- Mint is the primary credential provider: it resolves, scopes, issues,
  rotates, revokes, and audits credential access.
- The substrate enforces filesystem, process, and network isolation.
- External services ultimately enforce their credential permissions.

Secrets never enter workflow configuration, event payloads, argv, transcripts,
or evidence bundles. Public installations may bind another credential provider
behind the same interface; Misty Step uses Mint.

One plane is one trust domain. Humans authenticate through OIDC where
available, with a local bootstrap administrator for self-hosting. Agents and
integrations use scoped service identities rather than shared operator tokens.
Authorization is capability-based (`read`, `draft`, `activate`, `operate`, and
`administer`) and may be scoped to the plane or a workflow. Multi-tenant
isolation inside one plane is not an initial goal.

## Budgets tell the truth

Spend is governed primarily by workflow because workflow is the product object.
The hierarchy is:

1. plane ceiling;
2. workflow daily, monthly, rate, and concurrency limits;
3. run-group allowance shared by the parent, dynamic children, verification,
   and remediation;
4. agent defaults and optional cross-workflow ceilings.

Controls are labeled `hard`, `admission-only`, or `advisory` according to what
the harness and provider can actually enforce. Cost is labeled `reported`,
`estimated`, or `unavailable`; missing cost is never treated as zero. Aggregates
show their coverage rather than laundering partial data into false precision.

## Side effects, recovery, and remediation

Agents may perform declared external side effects directly. The plane does not
insert deterministic publisher services or scribe agents unless evidence shows
they earn their complexity. Side effects remain scoped by the agent ceiling and
workflow grant and produce inspectable receipts.

Infrastructure failures before harness execution may retry mechanically. Once
agent execution begins, Bitterblossom never blindly retries because external
effects may already exist. After worker loss, the plane probes and reattaches or
resumes when the substrate supports it. Ambiguous execution becomes
`needs_attention`, never an optimistic rerun.

Verification failure selects a declared remediation policy: stop, recommission,
or escalate. Recommissioning creates a new linked step attempt with explicit
round, time, and spend guards. Manual replay always creates a new linked run and
lets the operator select the original pinned revision or the currently active
revision.

## Evidence is the product

Every run produces an immutable evidence bundle containing the redacted trigger
payload, goal and configuration revisions, state timeline, parent and child
tree, harness events, tool activity, transcript, costs, budget decisions,
artifacts, external receipts, domain result, and verification result.

Queryable metadata lives in the database. Heavy transcripts and artifacts live
behind a blob-storage interface with configurable retention. Bundles export to
external evaluation and observability systems; Bitterblossom supplies execution
evidence but does not become the lab that declares one model or agent superior.

Only pre-effect failures are mechanically replayable. Notification delivery,
stale work, broken triggers, verification failure, exhausted guards, and budget
stops are durable operator facts rather than logs to discover later.

## The operator experience

The primary view is the configured workflow roster, not a metric dashboard and
not an activity feed. It answers what is configured to run before showing what
ran recently.

Each workflow makes its state, trigger, compact topology, assigned agents,
active instances, latest results, and budget visible without expansion. A
workflow detail keeps one stable configured graph and overlays aggregate live
counts. Selecting a run reveals only that run's path, timing, evidence, cost,
and dynamic child tree.

The interface never says a workflow itself is “running.” It separates:

- workflow configuration state;
- trigger health or schedule;
- run lifecycle;
- the domain result of one run;
- the verification result of one run.

Global secondary views are Agents, Runs, and Spend. Agents defaults to reusable
Roster identities in use, with unbound catalog entries available separately.
Runs defaults to live work before history. Triggers remain visibly attached to
workflows instead of becoming an independent inventory.

Workflow creation is goal-first and both conversational and visual. A
workflow-designer agent proposes a draft from natural-language intent while the
real editable graph, agent revisions, authority, budgets, and routes remain
inspectable. Activation normally requires mechanical preflight plus one real
test run against a fixture or test-scoped destination.

## Golden workflow: pull-request review

The first workflow to make excellent is PR Review. A reviewable pull-request
head creates one run, deduplicated by repository, pull request, and head SHA.
Drafts do not run. New commits supersede stale work; pending heads coalesce to
the newest SHA.

Cerberus receives the enhanced review goal and may compose Roster-defined or
ephemeral QA and specialist children. It rechecks the head SHA immediately
before posting one formal GitHub review:

- `clear` -> `APPROVE`;
- `blocked` -> `REQUEST_CHANGES`;
- `inconclusive` -> `COMMENT`;
- `superseded` -> no post.

Cerberus may read code and submit reviews. It may not push, merge, mutate
issues, or perform unrelated repository writes. A fresh verifier then judges
the original goal against Cerberus's transcript, child traces, inspected
evidence, and posted review. During hardening, every run receives mechanical
and independent verification. Verification failure follows the configured
stop, recommission, or escalation policy.

This workflow is not ready until real fixture pull requests prove blocking and
clear reviews, verification, deduplication, supersession, Mint scoping,
forbidden authority, budget stops, recovery, complete observability, and
remediation.

## Second workflow: Canary incident resolution

Canary incident resolution is the first full remediation loop:

> incident event -> claim -> investigate and annotate -> repair and release ->
> independently verify -> post-mortem and close

The responder queries Canary's durable incident context and timeline rather
than trusting a webhook payload. It claims the incident before work and writes
annotations as evidence is acquired. It may use service-scoped repository and
deployment authority, but it may not disable or weaken monitoring.

A fresh verifier reproduces the original symptom, checks regressions and
monitoring integrity, and confirms real recovery. Failure routes back to
remediation within configured guards. Success records a post-mortem,
`fix-verified` annotation, and verified remediation claim. Canary detects the
underlying recovery and emits `incident.resolved`; Bitterblossom does not fake
that state.

## What this repo refuses

- No supervised fleet-observation product until unattended workflows are
  excellent.
- No task-first dashboard or blank node-editor as the primary mental model.
- No semantic workflow engine or workload-specific judgment in Rust.
- No deterministic keyword heuristics where an agent must judge meaning.
- No resident persona fleet baked into the runtime.
- No built-in PR-review, incident, or domain brain; those remain agent goals.
- No dashboard-only configuration shadowing the API and database.
- No Git requirement for a self-hosted operator.
- No hidden authority escalation or shared operator credentials for agents.
- No cost labeled as enforced or known when the provider surface cannot prove it.
- No blind replay after agent execution begins.
- No GitHub-only event or artifact architecture.
- No substrate identity disguised as product architecture.
- No all-in-one security, evaluation, or observability platform accreting around
  the workflow spine.

## What excellent looks like

Near term, only PR Review is active while the workflow model and operator UI are
hardened. Existing run evidence is preserved; CI Audit, Powder Chew, Self Drill,
and the report-only Canary workflow are archived rather than mechanically
translated into the new model.

Medium term, Canary returns as the full incident-resolution loop. Additional
workflows are added one at a time, each earning new primitives through real
operational need rather than speculative generality.

Long term, Bitterblossom remains a small, understandable deterministic plane
around excellent specialist agents. Another operator can install it, connect a
Roster catalog and credential provider, create a workflow conversationally,
prove it against a fixture, activate it, and understand the fleet without
reading logs or inheriting Misty Step conventions.

## Where it sits

Roster provides reusable agent identities and launch contracts. Mint brokers
credentials. Canary provides production-health truth and responder
coordination. Powder provides the durable work ledger. Substrates execute
isolated work. Bitterblossom composes those capabilities into recurring,
budgeted, inspectable workflows.

The detailed product contract is `docs/workflow-control-plane.md`.
`docs/spine.md` documents the currently shipped file-defined runtime during the
migration. `project.md` records the historical 2026-06-10 pivot and this newer
direction lock.
