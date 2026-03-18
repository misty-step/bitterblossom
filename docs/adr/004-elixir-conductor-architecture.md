# ADR-004: Elixir Conductor — Behaviour-Driven Agent Orchestration

- **Status:** Accepted
- **Date:** 2026-03-14
- **Related:** ADR-001 (Claude Code harness), ADR-002 (thin CLI), ADR-003 (conductor control plane)

## Context

The Python conductor (`scripts/conductor.py`, 6,506 LOC) proved the Bitterblossom concept
but accumulated architectural debt: monolithic orchestration, broken revision loop (#607),
zero stdout (#608), and tight coupling to sprites, GitHub issues, and Claude Code.

A factory audit on 2026-03-14 confirmed: the Python conductor can't complete runs with PR
thread findings. An Elixir rewrite (1,649 LOC) merged PR #611 on its first attempt.

OpenAI's Symphony (Elixir) validates the architecture: OTP supervision, 5-state machines,
workspace isolation, and agent-trusting orchestration.

## Decision

**Replace the Python conductor with an Elixir/OTP application built on three Ousterhout
principles: deep modules, behaviours as extension points, and the conductor as authority
(lease + merge) while agents own judgment (code + reviews).**

### Core Behaviours

The conductor is pluggable at four boundaries. Each boundary is an Elixir behaviour
with 2-4 callbacks. New implementations require zero orchestrator changes.

```elixir
# Where agents run
defmodule Conductor.Worker do
  @callback exec(config, command, opts) :: {:ok, binary()} | {:error, term()}
  @callback dispatch(config, prompt, repo, opts) :: {:ok, binary()} | {:error, term()}
  @callback cleanup(config, run_id) :: :ok | {:error, term()}
end
# Implementations: Worker.Sprite, Worker.Docker, Worker.SSH, Worker.Local

# Where work comes from
defmodule Conductor.Tracker do
  @callback list_eligible(config, opts) :: {:ok, [Issue.t()]}
  @callback get_issue(config, id) :: {:ok, Issue.t()}
  @callback comment(config, id, body) :: :ok | {:error, term()}
  @callback transition(config, id, state) :: :ok | {:error, term()}
end
# Implementations: Tracker.GitHub, Tracker.Linear, Tracker.Jira

# How agents think
defmodule Conductor.Harness do
  @callback name() :: binary()
  @callback dispatch_command(opts) :: {binary(), [binary()]}
  @callback parse_artifact(binary()) :: {:ok, map()} | {:error, term()}
end
# Implementations: Harness.ClaudeCode, Harness.Codex, Harness.OpenCode

# Where PRs live (decoupled from tracker)
defmodule Conductor.CodeHost do
  @callback create_pr(config, branch, opts) :: {:ok, map()} | {:error, term()}
  @callback checks_green?(config, pr) :: boolean()
  @callback merge(config, pr, opts) :: :ok | {:error, term()}
end
# Implementations: CodeHost.GitHub, CodeHost.GitLab
```

### Supervision Tree

```text
Conductor.Application
├── Conductor.Store (GenServer — SQLite persistence)
├── Conductor.EventBus (Phoenix.PubSub — event broadcasting)
├── Conductor.RunSupervisor (DynamicSupervisor — one GenServer per run)
├── Conductor.Orchestrator (GenServer — polling + dispatch)
├── Conductor.Telemetry (GenServer — cost tracking + metrics)
└── ConductorWeb.Endpoint (Phoenix — LiveView dashboard, optional)
```

### Run State Machine

```text
pending → building → governing → terminal
                                   ├── merged
                                   ├── blocked
                                   └── failed
```

Four phases, not ten. The builder agent owns implementation + revision internally.
The conductor only gates authority decisions: lease, merge, block.

### Event-Driven Architecture

All state transitions emit events via `Phoenix.PubSub`. The Store, Dashboard,
Telemetry, and any future sidecar subscribe independently. No polling from
internal consumers — only the orchestrator polls the external tracker.

```elixir
Conductor.EventBus.broadcast({:run, run_id}, {:phase_changed, :building, metadata})
```

### Dashboard (Phoenix LiveView)

Real-time operator surface. No React, no API layer, no WebSocket library.
LiveView renders server-side and streams diffs over a single WebSocket.

Pages:
- **Fleet** — Worker health, current assignments, probe history
- **Runs** — Active and completed runs with event timelines
- **Costs** — Token usage, estimated costs, budget gates
- **Backlog** — Queued issues, readiness, routing decisions
- **PRs** — Open PRs, CI status, review state, merge readiness
- **Bakeoffs** — Model/harness comparison on identical tasks

### Model Bakeoffs

The orchestrator can dispatch the same issue to N workers with different
model/harness configurations. Results are compared on:
- Time to completion
- Token usage / cost
- Test pass rate
- Review finding count
- Lines changed

This is a first-class feature, not an afterthought. The `Conductor.Bakeoff`
module manages parallel runs and the LiveView dashboard renders comparisons.

### Cost Tracking

Every dispatch records model, provider, input/output tokens, and estimated cost.
The telemetry module aggregates per-run, per-model, per-day, and per-issue.
Budget gates can pause the orchestrator when spend exceeds thresholds.

## Rationale

1. **Behaviours > configuration.** Adding a new worker backend is implementing
   4 callbacks, not adding flags to a monolithic function. Ousterhout: define
   errors out of existence by making the type system enforce the contract.

2. **OTP > threads.** Each run is a process with its own lifecycle. Crashes are
   isolated. Supervision restarts cleanly. No global state corruption.

3. **LiveView > SPA.** The dashboard is server-rendered with real-time updates.
   No API layer, no frontend build, no state synchronization bugs. One language
   for the entire stack.

4. **Events > polling.** Internal consumers subscribe to PubSub. The dashboard
   updates in real-time without polling the database.

5. **Agent trust > micro-orchestration.** The builder handles reviews internally.
   6,506 → 1,649 LOC. The complexity was in orchestrating what the agent already
   knows how to do.

## Consequences

- Python conductor is deprecated. It remains as reference but new features land in Elixir.
- `bb` CLI (Go) remains as transport — it's thin and correct.
- Sprites remain as the primary worker backend, with Docker/SSH as future options.
- GitHub remains as the primary tracker, with Linear/Jira as future options.
- Phoenix is added as a dependency for LiveView dashboard and PubSub.
- Elixir runtime is required on the coordinator (sprite or local).

## Migration Path

1. **Phase 1 (done):** Core conductor with Store, RunServer, Orchestrator, CLI
2. **Phase 2:** Extract behaviours, add EventBus, add Telemetry
3. **Phase 3:** Phoenix LiveView dashboard
4. **Phase 4:** Bakeoff support, cost tracking
5. **Phase 5:** Additional Worker/Tracker/Harness implementations
