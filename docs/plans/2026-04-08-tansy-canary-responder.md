# Context Packet: Tansy, Canary Incident Responder

## Why This Exists

Bitterblossom is currently shaped as a single-repo, backlog-driven factory.
The immediate direction lock is different: one always-on sprite should watch
Canary across all reporting services, investigate what is actually happening,
fix the right repository, review the change rigorously, merge, deploy, verify
recovery, and continue monitoring.

Canary already provides the right observability substrate for this:

- signed webhook hints
- canonical read APIs
- incidents
- timelines
- annotations

What Bitterblossom lacks is a dedicated responder lane, a truthful mapping from
Canary `service` to repository and allowed actions, and an incident-response
skill path that is built for live production problems instead of backlog items.

## Product Exploration

### Approach 1: Reuse `triage` and point Muse at Canary

Keep `role = "triage"` and try to make a fleet persona act like a Canary
responder.

Why reject it:

- `triage` is hard-wired to `muse` in loader, workspace, and launcher wiring.
- Muse currently observes and recommends; it does not implement, merge, or
  deploy.
- The semantic mismatch would be encoded as prompt folklore instead of a clear
  system contract.

### Approach 2: Add a first-class responder lane and keep Canary polling as truth

Introduce a dedicated responder identity, `Tansy`, with a responder skill that
polls Canary read APIs, claims incidents through annotations, resolves the
target repo through a typed catalog, investigates strategically, runs review,
and optionally merges and deploys for allowlisted services.

Recommendation: this is the right v1.

Why:

- matches the actual job
- keeps Canary as observability substrate rather than workflow engine
- avoids public webhook infrastructure before it is needed
- makes the safety boundary explicit
- stays deletable and evolvable

### Approach 3: Build webhook intake into Bitterblossom first

Expose a public Bitterblossom endpoint, subscribe it to Canary webhooks, and
drive the responder from pushed events.

Why defer it:

- Canary explicitly treats webhooks as wake-up hints, not source of truth.
- Bitterblossom does not currently need public ingress to solve the real
  problem.
- It adds queueing, signature verification, reachability, and replay concerns
  before the responder behavior itself exists.

## Goal

One always-on Bitterblossom sprite, `Tansy`, autonomously handles active Canary
incidents for allowlisted services end to end: claim, investigate, fix, review,
merge, deploy, confirm recovery, annotate, repeat.

## Non-Goals

- Do not build a generic event bus or workflow console into Canary.
- Do not start with a public webhook receiver in Bitterblossom.
- Do not horizontally scale responders in v1.
- Do not treat raw error groups as the primary work queue in v1.
- Do not encode arbitrary shell workflow logic as an open-ended DSL.
- Do not grant merge or deploy authority to every service by default.

## Constraints / Invariants

- Canary read APIs are the source of truth. Webhooks are hints only.
- `Tansy` must be a first-class persona path. Do not overload `triage`/`muse`.
- Exactly one active `Tansy` loop in v1.
- v1 work queue is active incidents first. Error groups are evidence, not the
  primary scheduler.
- Service-to-repo resolution must live in Bitterblossom, not Canary.
- The service catalog must be typed and validated at startup.
- Catalog commands are argv arrays, not shell strings.
- Auto-merge and auto-deploy are explicit per-service opt-ins and default to
  `false`.
- A fix is not done until Canary stays healthy through a stabilization window.

## Authority Order

tests > Canary API contracts > type system > code > docs > lore

## Repo Anchors

- `conductor/lib/conductor/fleet/loader.ex` — valid roles are explicit and must
  stay truthful.
- `conductor/lib/conductor/workspace.ex` — persona sync is role-driven; Tansy
  needs first-class wiring.
- `conductor/lib/conductor/launcher.ex` — loop prompt, persona display, and
  workspace lifecycle.
- `fleet.toml` — fleet declaration and sprite authority surface.
- `sprites/muse/AGENTS.md` — current `triage` semantics that this plan must not
  silently overload.
- `../canary/lib/canary_web/router.ex` — current read and annotation API
  surface.
- `../canary/lib/canary_web/controllers/annotation_controller.ex` — current
  annotation write semantics and race boundary.
- `../canary/README.md` — webhook and polling contract.

## Prior Art

- `conductor/test/conductor/canary_integration_test.exs` — Bitterblossom
  already attaches itself to Canary for outbound reporting.
- `../canary/README.md` — canonical statement that webhooks are wake-up hints.
- `../canary/test/canary_web/controllers/incident_controller_test.exs` —
  incident filtering via annotations already exists.
- `../canary/test/canary/query_annotation_test.exs` — error-group annotation
  filtering already exists for evidence gathering.

## Product Shape

### Sprite identity

- Sprite name: `bb-tansy`
- Persona name: `tansy`
- Role name: `responder`

Do not reuse `triage`. The current code and persona tree do not support that
cleanly.

### Work queue

`Tansy` handles:

1. `GET /api/v1/incidents`
2. `GET /api/v1/report`
3. `GET /api/v1/timeline`
4. focused reads such as `GET /api/v1/errors/:id` and health check history

Scheduling rule:

- v1 picks from active incidents without a `bitterblossom.claimed` annotation.
- Error groups may be queried while investigating, but they do not independently
  trigger repo mutation in v1.

### Coordination protocol

Canary annotations are the operator-visible state trail:

- `bitterblossom.claimed`
- `bitterblossom.investigating`
- `bitterblossom.resolved`
- `bitterblossom.escalated`
- `bitterblossom.rollback`

Each annotation includes:

- `agent`
- `action`
- metadata with `sprite`, `run_id`, `repo`, `branch`, `started_at`, and
  `expires_at`

Because annotation claims are not atomic, v1 relies on the stronger invariant:
single active Tansy only. No second responder until claim semantics are
hardened.

### Service catalog

Add a typed catalog at repo root:

`canary-services.toml`

Minimal v1 schema per service:

```toml
[[service]]
name = "volume"
repo = "misty-step/volume"
default_branch = "main"
test_cmd = ["make", "test"]
deploy_cmd = ["flyctl", "deploy", "--app", "volume-prod", "--remote-only"]
rollback_cmd = ["flyctl", "releases", "rollback", "--app", "volume-prod"]
auto_merge = false
auto_deploy = false
stabilization_window_s = 600
```

Rules:

- strict schema validation on load
- no shell interpolation
- no optional branching language
- defaults are deny-by-default

### Incident execution flow

The hot path should be investigation-first, not backlog-first:

1. Poll Canary for eligible incidents.
2. Claim the incident and annotate `bitterblossom.claimed`.
3. Gather canonical context from incident, report, timeline, error detail, and
   target-check APIs.
4. Resolve the target repo from `canary-services.toml`.
5. Check out a dedicated worktree for that repo.
6. Run `/investigate`.
7. Brainstorm and validate strategic fixes with `/research thinktank` when the
   design is unclear or the fix is risky.
8. Implement the chosen fix with tests.
9. Run `/code-review` so the thinktank review tier is mandatory.
10. Apply `/settle`-style verification gates.
11. If `auto_merge=true`, squash merge after all review and verification gates.
12. If `auto_deploy=true`, run the typed deploy command.
13. Confirm Canary recovery through the stabilization window.
14. Annotate `resolved`, `rollback`, or `escalated`.

`/autopilot` is not the default primitive here. It is backlog-oriented. The
incident lane should be `/investigate` first, then focused fix, then
`/code-review`, then settlement and rollout gates.

## Technical Exploration

### Option A: Dedicated responder role + catalog + polling/reconciliation

Add explicit responder wiring in the conductor and keep responder behavior in a
small set of deep modules and persona-specific skills.

Recommendation: choose this.

Why:

- small conductor change, large semantic clarity gain
- no public ingress required
- matches Canary’s actual contract

### Option B: Reuse `triage` and patch Muse into Tansy

Why reject it:

- wrong persona wiring
- wrong operator language
- hidden coupling between unrelated workflows

### Option C: Public webhook intake + queue before responder behavior

Why reject it for v1:

- infrastructure before product
- more failure modes than value at this stage

## Recommended Technical Design

### 1. Add a first-class responder role

Extend:

- `Conductor.Fleet.Loader` valid roles
- `Conductor.Workspace` persona role whitelist and role mapping
- `Conductor.Launcher` role display name
- focused conductor tests

Add:

- `sprites/tansy/CLAUDE.md`
- `sprites/tansy/AGENTS.md`

### 2. Add a dedicated incident-response skill

Add a repo-local skill such as:

- `base/skills/canary-responder/SKILL.md`

This skill owns the responder loop contract:

- poll
- claim
- context gather
- investigate
- fix
- review
- merge gate
- deploy gate
- recovery gate
- annotate

Do not bury this protocol in giant AGENTS prose.

### 3. Keep the runtime thin with four deep modules

- `CanaryInbox`
  - `next_incident/1`
  - `incident_context/2`
- `ServiceCatalog`
  - `load!/1`
  - `resolve!/2`
- `IncidentLease`
  - `claim/2`
  - `heartbeat/2`
  - `close/2`
- `Remediator`
  - `execute/2`

If these land as conductor modules, they should stay narrow and truthful. If
they land as skill-owned scripts first, keep the same interface boundaries.

### 4. Explicit incident lifecycle

Per work item:

`new -> claimed -> investigating -> fixing -> review -> verified -> deployed -> recovered`

Terminal states:

- `escalated`
- `rollback`

### 5. Merge and deploy authority

This plan assumes:

- `Tansy` may merge and deploy, but only for services explicitly allowlisted in
  the catalog.
- services without `auto_merge` and `auto_deploy` stay investigation-only or
  merge-ready with escalation.

That keeps the user’s desired end-to-end automation while preserving a hard
safety boundary.

## Oracle (Definition of Done)

- [ ] `cd conductor && mix test test/conductor/fleet/loader_test.exs test/conductor/workspace_test.exs test/conductor/launcher_test.exs`
- [ ] `cd conductor && mix test test/conductor/canary_integration_test.exs test/conductor/tansy_catalog_test.exs test/conductor/tansy_lease_test.exs test/conductor/tansy_responder_test.exs`
- [ ] `cd conductor && mix test test/conductor/tansy_e2e_test.exs`
- [ ] `make test`
- [ ] In the fixture e2e test, one seeded Canary incident is claimed exactly
  once, mapped to the right repo, annotated through `claimed -> investigating ->
  resolved|escalated`, and never double-processed.
- [ ] For an allowlisted fixture service, merge only happens after `/code-review`
  returns a ship verdict and the repo verification command exits `0`.
- [ ] For an auto-deploy fixture service, Canary stays healthy for the full
  stabilization window before the incident is marked resolved.

## Implementation Sequence

1. Lock the product pivot in docs.
   Update `project.md`, `CLAUDE.md`, and any workflow text that still frames
   Bitterblossom as single-repo only.
2. Introduce `responder` + `tansy` persona wiring and tests.
3. Add `canary-services.toml` plus strict loader/validator tests.
4. Add the `canary-responder` skill and Tansy persona loop.
5. Implement polling, annotation, and repo-resolution flow for incidents only.
6. Add focused integration tests against mocked Canary APIs and a fixture target
   repo.
7. Enable `bb-tansy` in `fleet.toml`; disable or retire `bb-muse` if the pivot
   is total.
8. Roll out on one allowlisted service with `auto_merge=false` and
   `auto_deploy=false`.
9. After real runs are trustworthy, selectively enable autonomous merge and
   deploy per service.

## Risk + Rollout

- Blast radius:
  auto-merge and auto-deploy are dangerous across many repos. Default deny, then
  opt in per service.
- Claim races:
  acceptable in v1 only because there is one active Tansy. Do not scale out
  until lease semantics are stronger.
- Rate limits:
  poll incidents and narrow report/timeline reads; do not sweep every service on
  every loop.
- False recovery:
  require a stabilization window before `resolved`.
- Repeated bad deploys:
  require `rollback_cmd` for any service allowed to auto-deploy.
- Product drift:
  current top-level docs still describe a different product. Update them before
  implementation starts.

## Open Questions

- Should merge authority live entirely in `Tansy`, or should `Tansy` hand off
  merge to a slimmer Fern lane after incident repair is ready?
- Should the idempotency ledger stay annotation-only in v1, or should
  Bitterblossom persist responder leases in `Conductor.Store` before the first
  rollout?
- Does the catalog belong at repo root, or under `docs/context/` once the pivot
  is fully codified?
