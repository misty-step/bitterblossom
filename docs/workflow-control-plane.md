# Workflow Control Plane Product Contract

Status: operator-ratified product specification, 2026-07-11. `VISION.md` owns
the durable premise; this document owns the detailed product and UX contract.

## Scope

Bitterblossom initially supports unattended, trigger-driven workflows only.
Supervised and interactive local-agent presence is deferred. Workflow is the
primary configured object and the primary dashboard object.

The smallest complete model is:

```text
trigger -> step(agent + goal) -> result -> route(next step | done)
```

Configured nouns are Workflow, Trigger, Step, Route, and Agent. Runtime nouns
are Run and Result. Verification, loops, handoffs, remediation, and effects are
compositions of those nouns rather than new engine concepts.

## Workflow definition

A workflow revision contains:

- name, natural-language goal, and accepted goal-enhancement revision;
- one or more normalized triggers;
- steps, each binding a pinned Roster agent revision to a goal and environment;
- named result outcomes only where deterministic routing needs them;
- routes to another step or completion;
- workflow and run-group policies for authority, budget, concurrency,
  verification, remediation, retention, and notification;
- a fixture or captured test event and the evidence from its activation test.

The database is authoritative for active configuration. Every edit is an
immutable revision. CLI and dashboard use one API. Declarative documents are
portable import/export and optional GitOps surfaces.

## Goal authoring

The create flow begins with “What should this workflow accomplish?” A shaping
agent proposes an enhanced natural-language commission with outcome, acceptance
evidence, boundaries, prohibited actions, context, assumptions, and ambiguity
handling. The operator explicitly accepts that revision.

The remaining sequence is:

```text
Goal -> Trigger -> Agent step -> Environment/tools/authority -> Limits -> Test -> Activate
```

A workflow-designer agent may propose the graph conversationally, but the real
graph and compiled configuration are always visible and directly editable.

## Results and routing

Successful completion of a single-route terminal step implies `completed`.
When routing needs a named outcome, the step receives a completion tool with:

- one outcome from the step's declared vocabulary;
- an unrestricted natural-language summary;
- optional artifact and external receipt references.

Missing or undeclared required outcomes are `incomplete`; deterministic code
does not infer them from prose. Structural facts may route directly. Semantic
decisions use an agent step.

Artifacts are optional evidence or handoff data. A result routes or emits a
normalized event and may reference artifacts; artifact existence by itself is
not a trigger.

Cycles select one or more guards: rounds, elapsed time, spend, deadline, or an
external stop signal. At least one guard must be genuinely enforceable for an
unattended cycle. The first fired guard applies the declared terminal policy.

## Agents and composition

Roster defines reusable identity, model policy, harness, instructions, skills,
and maximum capability envelope. Bitterblossom synchronizes catalog revisions,
stores immutable materialized snapshots, and pins them at workflow activation.

The workflow binding supplies goal, context, environment, tools, credentials,
and a grant no broader than the Roster ceiling. Dynamic children inherit a
narrower grant and appear only in the parent run tree. Their goal, model,
harness, tools, authority, cost, duration, and result remain in evidence.

Creating an agent inside the workflow builder creates a reusable local
Roster-compatible catalog entry. Roster needs a package-quality loading,
validation, and materialization API to support this relationship.

## Runtime state

The UI keeps these facts separate:

| Layer | States or values |
|---|---|
| Workflow | draft, active, paused, archived |
| Trigger | listening, scheduled, delayed, broken |
| Run | queued, preparing, executing, succeeded, failed, cancelled |
| Step result | workflow-defined outcome plus natural-language evidence |
| Verification | not required, pending, achieved, not achieved, inconclusive |

Domain results belong to runs, never to the workflow definition. A summary may
say “latest run succeeded; verdict blocked,” but the workflow itself is not
blocked by that domain verdict.

Pausing suppresses new runs and records incoming-event disposition. Active runs
continue unless explicitly cancelled. Resume does not automatically replay
suppressed events. Activated definitions are archived rather than deleted.

## Concurrency and revisions

Every accepted event pins the workflow and agent revisions then active. New
activations affect new events only. Rollback activates an old snapshot as a new
revision.

Workflow-specific concurrency policy may serialize or coalesce on a declared
key. PR Review is single-flight per pull request: duplicate head events dedupe,
pending heads coalesce to the newest, and active stale work posts nothing after
its final head check.

## Verification and remediation

An agent's success is a completion claim. Verification policy may be:

- execution-only;
- mechanical checks;
- independent critic;
- mechanical checks plus independent critic.

A critic receives the original goal, configuration, transcript, child traces,
outputs, and external receipts. It is read-only and returns achieved, not
achieved, or inconclusive with evidence.

Verification failure selects stop, recommission, or escalate. Recommissioning
is a new linked attempt because side effects may already exist. It shares the
originating run-group guards and budget.

## Recovery

Pre-execution infrastructure failure may retry mechanically. Once harness
execution begins, the plane never blindly retries. It probes and reattaches or
resumes where supported; otherwise it records `needs_attention` with the exact
uncertainty. Manual replay creates a new linked run and explicitly selects
original or current revisions.

## Authority, credentials, and access

Authority narrows from Roster ceiling to workflow grant to child grant. Mint is
the primary credential provider. Secret values never enter workflow config,
payloads, argv, transcripts, or stored evidence.

One plane is one trust domain. Humans use OIDC where available; agents and
connectors use scoped service identities. Capabilities are read, draft,
activate, operate, and administer, scoped to the plane or workflow. Viewer,
editor, operator, and administrator are presets rather than architectural
roles. Multi-tenant isolation within one plane is deferred.

## Budget and cost

Budgets attach primarily to plane, workflow, and run group. Agent defaults and
optional cross-workflow ceilings supplement them. Dynamic children,
verification, and remediation consume the same originating run-group allowance.

Every limit says whether it is hard, admission-only, or advisory. Every cost is
reported, estimated with a pinned rate card, or unavailable. Aggregates expose
coverage and never treat unknown as zero.

## Evidence and history

Each run has an immutable evidence bundle:

- normalized trigger and redacted payload;
- workflow, goal, and agent revisions;
- state-transition timeline;
- parent and child run tree;
- harness stream, tool activity, and transcript;
- costs, usage, duration, and budget decisions;
- artifacts and external receipts;
- domain result and verification result.

The database indexes metadata. A blob interface stores heavy evidence under a
configurable retention policy. Bundles export to external evaluation systems;
Bitterblossom does not own experimental design or model promotion.

## Dashboard information architecture

The landing page is the configured workflow roster. It shows each workflow's
configuration state, trigger in plain language, compact topology, assigned
agents, active run counts and current steps, latest run summary, and budget
position.

Workflow detail keeps one stable configured graph. Aggregate live counts sit on
nodes. An adjacent run rail selects one current or historical instance and
overlays only its path, timings, cost, results, artifacts, and dynamic children.

Global secondary views are:

- Agents: In use by default, Available separately;
- Runs: Live by default, History separately;
- Spend: attribution and controls by workflow, run, step, and agent.

Schedules show timezone and next fire times. Event triggers show source,
filters, connection health, last accepted event, and dispositions for accepted,
filtered, duplicate, and rejected events. The UI never invents a next run for
event-driven work.

Analytics emphasize execution success, verified achievement, domain outcomes,
cost per verified achievement, queue and execution latency, remediation rounds,
supersession, and failure concentration. No synthetic fleet-health score hides
the underlying facts.

Notifications are exception-first and link directly to the workflow, run,
step, and evidence that need attention.

## Golden workflow: PR Review

One automatic run is admitted for each reviewable pull-request head. Draft
updates and metadata noise do not run. Cerberus may compose QA and specialist
children, then directly submits one formal review after rechecking the head:

| Result | GitHub review state |
|---|---|
| clear | APPROVE |
| blocked | REQUEST_CHANGES |
| inconclusive | COMMENT |
| superseded | no post |

Cerberus has review/comment authority only. A fresh verifier then evaluates the
goal, execution evidence, and posted review. During hardening, mechanical and
critic verification run on every instance. Release requires real fixture drills
covering clear and blocking reviews, dedupe, supersession, authority denial,
Mint scoping, budgets, recovery, observability, and remediation.

## Second workflow: Canary incident resolution

The workflow claims an incident, queries Canary's durable responder context,
logs annotations while investigating, repairs and releases with service-scoped
authority, independently verifies recovery and instrumentation integrity, and
records the post-mortem plus verified remediation claim. Failure routes back to
remediation under configured guards. Canary, not Bitterblossom, emits the
incident-resolved state when the monitored condition genuinely recovers.

## Migration

Preserve existing run evidence. Archive current Canary Triage, CI Audit, Powder
Chew, and Self Drill configurations. PR Review is the only active workflow while
the new runtime and UI harden. Remove obsolete file-defined workload and example
deadwood after moving any unique mechanism coverage into generic fixtures.

Reintroduce Canary through the full incident-resolution workflow. Add later
workflows one at a time so new mechanism is earned by a real operational need.
