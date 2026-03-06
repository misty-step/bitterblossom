# ADR-003: Remote Conductor Control Plane

- **Status:** Accepted
- **Date:** 2026-03-06
- **Related:** ADR-001 (Claude Code as canonical harness), ADR-002 (Thin CLI, Thick Skills)

## Context

Bitterblossom currently provides a thin remote transport:

- `setup` provisions a sprite and clones a repo
- `dispatch` runs the Ralph loop against a sprite
- `status`, `logs`, and `kill` provide operator recovery

That is enough for manual task dispatch, but not enough for a 24/7 software factory. The missing layer is not "more `dispatch` flags." It is a control plane that owns:

- intake from GitHub issues and Sentry-derived issues
- prioritization and routing
- durable leases
- run state and heartbeats
- review council orchestration
- CI and merge gating
- experiment logging for model/profile comparisons

The user also wants optional parallel implementation variants: multiple workers can implement the same spec with different profiles, then a reviewer council can select or synthesize the strongest result.

## Decision

**Bitterblossom will remain the conductor, but the conductor will run remotely as an always-on control plane.**

`bb` stays a thin operator and transport CLI. It is not expanded into a large workflow brain. The 24/7 orchestration loop runs on a dedicated coordinator sprite or another always-on host, not on a developer laptop.

The architecture is split into four layers:

1. **Intake**
2. **Router**
3. **Worker Runtime**
4. **Governance**

Everything else is implementation detail.

## Architecture

### 1. Intake

GitHub is the system of record for work.

- Primary feed: GitHub issues
- Secondary feed: Sentry incidents converted into GitHub issues with dedupe and context
- Human overrides live in GitHub labels, assignees, issue state, and comments

Sentry is not treated as a second queue. It is an issue-enrichment source.

### 2. Router

The router selects the next issue and the execution profile.

- Hard filters are deterministic: open, eligible, not leased, not blocked, dependencies satisfied
- Ranking is judgment-heavy and belongs to LLM reasoning, not static heuristics
- Routing chooses both a sprite and a profile

Profiles capture runtime differences without branching the whole system:

- harness
- model
- provider
- persona
- prompt pack
- tool mounts
- budget and timeout policy

### 3. Worker Runtime

The runtime interface stays small:

- `Start(run)`
- `Stream(run)`
- `Checkpoint(run)`
- `Stop(run)`

The current `bb` transport is a valid implementation of this layer.

Workers are persistent for caches and warm repos, but runs are isolated.

- Keep a persistent repo mirror and dependency caches on each sprite
- Create a fresh worktree per run
- Never reuse a dirty shared checkout as the execution surface
- Record run metadata in the workspace as a first-class contract

### 4. Governance

Governance is separate from building.

- review council
- CI wait and retry loop
- merge gate
- stale lease recovery
- blocked run escalation

The builder should not decide that it is done because it feels done. Completion is a control-plane decision backed by explicit state.

## Remote vs Local

The production conductor runs remotely.

Reasons:

- laptops sleep
- shells drift
- tokens expire silently
- cron and webhook handling need 24/7 uptime
- reconciliation loops should not depend on human presence

Local `bb` remains essential for:

- bootstrap
- debugging
- manual recovery
- shakedowns
- ad hoc dispatch

## Run State

Every execution gets a durable `run_id` and explicit phase.

Minimum phases:

- `queued`
- `leased`
- `building`
- `reviewing`
- `revising`
- `ci_wait`
- `merge_ready`
- `merged`
- `blocked`
- `failed`

Each run stores:

- issue id
- sprite
- profile id
- branch
- PR number
- heartbeat timestamp
- checkpoint ids
- council verdicts
- artifact paths

GitHub remains the human-facing source of truth. The run store is the machine-facing source of truth.

## Parallel Variants

Parallel implementation variants are supported, but not mandatory.

- default breadth is `1`
- higher breadth is reserved for ambiguous or high-value work
- each variant uses its own profile and isolated worktree
- the review council can choose a winner or request a synthesis run

This is an optimization layer, not the baseline path.

The control plane must log variant outcomes so profile selection can improve over time.

## Observability

Observability is run-centric, append-only, and operator-readable.

Minimum requirements:

- durable run store
- structured event log
- heartbeat timestamps
- remote log streaming
- checkpoint history
- explicit workspace metadata

Operator commands should report what is true, not collapse distinct failures into generic strings.

## MVP

The MVP is a single-repo factory with:

- one remote conductor
- GitHub issue intake
- Sentry-to-GitHub issue creation
- one active builder per issue
- one review council with three independent reviews
- required green CI before merge
- SQLite-backed run state

This is enough to prove the full loop:

- pick issue
- lease issue
- dispatch to worker
- open PR
- review
- revise
- wait for green CI
- merge
- close run

## Consequences

- Do not turn `dispatch` into a large workflow engine
- Add durable contracts at the edges instead of more shell heuristics
- Prefer remote conductor services or sessions over laptop automation
- Treat parallel variants as an explicit routing policy, not the default for all tasks
- Fix observability lies and hidden coupling as part of the conductor foundation
