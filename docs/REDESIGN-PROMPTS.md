# Phase 2 Codex Dispatch Prompts

These prompts are ready to fire after Phase 1 (bash→Go port) merges.

---

## Agent 1: Core Abstractions
**Worktree:** `/tmp/core-abstractions`

```
You are implementing the core type system for the Bitterblossom sprite factory CLI.

Read docs/REDESIGN.md for the full architecture vision. You are building the FOUNDATION that all other packages depend on.

## YOUR TASK
Create the core abstractions package with deep module design (Ousterhout):

1. `internal/sprite/sprite.go` — Sprite type with state machine
   - States: provisioned, idle, working, done, blocked, dead
   - Only valid transitions allowed (enforce at type level)
   - Sprite is an OPAQUE HANDLE — callers never see VM details
   - Methods: Provision(), Dispatch(task), Signal(signal), Teardown()
   - Each method is a deep module: simple interface, handles all complexity internally

2. `internal/sprite/lifecycle.go` — State transition logic
   - Define errors out of existence: idempotent transitions
   - provision existing sprite = verify + no-op
   - teardown nonexistent sprite = success
   - dispatch to busy sprite = queued or error (configurable)

3. `internal/fleet/fleet.go` — Fleet type
   - Holds []Sprite + Composition (desired state)
   - Fleet.Reconcile() → []Action (what needs to change)
   - Fleet.Status() → FleetReport (current state summary)

4. `internal/fleet/composition.go` — YAML composition parser
   - Reads compositions/*.yaml
   - Resolves sprite personas from sprites/ directory
   - Validates composition (no duplicate names, valid personas, etc.)

5. `pkg/events/event.go` — JSONL event types
   - Event interface with Timestamp, Sprite, Kind
   - Concrete types: ProvisionEvent, DispatchEvent, ProgressEvent, DoneEvent, ErrorEvent
   - Marshal/Unmarshal to JSONL

6. Full test coverage for all packages (table-driven tests)

## DESIGN CONSTRAINTS
- Deep modules: simple interfaces, complex implementations
- Information hiding: Sprite internals are private
- Define errors out of existence: idempotent operations
- All state transitions explicit and validated
- No external dependencies yet (no Fly.io, no git) — use interfaces
- Types should be usable by all other packages

Commit each package separately with clear messages.
When completely finished, run: openclaw gateway wake --text "Done: Phase 2 core abstractions — Sprite, Fleet, Composition, Event types" --mode now
```

---

## Agent 2: Fleet Reconciler
**Worktree:** `/tmp/fleet-reconciler`

```
You are implementing the fleet reconciliation engine for Bitterblossom.

Read docs/REDESIGN.md for the full architecture. You are building the KUBECTL-STYLE reconciler that converges actual fleet state to desired state.

## YOUR TASK

1. `internal/fleet/reconcile.go` — The reconciliation engine
   - Input: Composition (desired) + []Sprite (actual from Fly.io)
   - Output: []Action (provision, teardown, update, redispatch)
   - Actions are DATA, not side effects — the caller decides whether to execute
   - Support --dry-run natively: reconcile always returns actions, execution is separate
   - Handle edge cases: sprite exists but wrong persona, sprite exists but wrong config version

2. `internal/fleet/actions.go` — Action types and executors
   - ProvisionAction, TeardownAction, UpdateAction, RedispatchAction
   - Each action has: Description() string, DryRun() string, Execute(ctx) error
   - Executor runs actions in correct order (teardowns before provisions to free resources)

3. `cmd/bb/compose.go` — The `bb compose` subcommand
   - `bb compose diff` — show what would change
   - `bb compose apply` — reconcile fleet (--dry-run default, --execute to apply)
   - `bb compose status` — show current composition vs desired
   - Pretty table output for humans, JSON for machines

4. `pkg/fly/client.go` — Fly.io API client (interface + real implementation)
   - Interface: MachineClient with Create, Destroy, List, Status, Exec methods
   - Real implementation using Fly.io Machines API (not CLI)
   - Mock implementation for tests
   - Retry with exponential backoff on transient errors

5. Full test suite — especially reconciliation edge cases

## DESIGN CONSTRAINTS
- Reconciliation is PURE: no side effects, only data
- Execution is SEPARATE: caller chooses to run or dry-run
- Fly.io interactions go through interface (testable)
- Define errors out of existence: reconcile an already-correct fleet = empty action list

Commit each package separately.
When completely finished, run: openclaw gateway wake --text "Done: Phase 2 fleet reconciler — compose apply, Fly.io client, action executors" --mode now
```

---

## Agent 3: Event Protocol
**Worktree:** `/tmp/event-protocol`

```
You are implementing the JSONL event protocol for Bitterblossom — the UNIX composability layer.

Read docs/REDESIGN.md for architecture context.

## YOUR TASK

1. `pkg/events/` — Complete event system
   - event.go: Event types (Provision, Dispatch, Progress, Done, Blocked, Dead, Health, PR)
   - emitter.go: JSONL emitter to io.Writer (default: stdout)
   - reader.go: JSONL reader from io.Reader (for piping between tools)
   - filter.go: Event filters (by sprite, by kind, by time range)
   - Full test coverage

2. `internal/watch/watcher.go` — The event-driven watchdog daemon
   - Polls fleet state at configurable interval
   - Detects signals: sprite done, sprite blocked, sprite dead, sprite stale
   - Emits events for each detection
   - In --execute mode: takes actions (auto-push, redispatch, alert)
   - In default mode: emits events only (UNIX: let the caller decide)

3. `internal/watch/signals.go` — Signal detection logic
   - Done: DONE file exists or specific git tag
   - Blocked: BLOCKED file with reason
   - Dead: claude process not running, no git activity in N minutes
   - Stale: running but no commits in 2 hours
   - Each signal is a pure function: (SpriteState) → Signal | nil

4. `cmd/bb/watch.go` — The `bb watch` subcommand
   - `bb watch` — emit events to stdout (JSONL)
   - `bb watch --execute` — also take actions
   - `bb watch --filter sprite=bramble` — filter output
   - `bb watch | bb alert` — pipe to alerting
   - Graceful shutdown on SIGTERM/SIGINT

5. `cmd/bb/logs.go` — The `bb logs` subcommand
   - `bb logs bramble` — tail sprite logs (real-time)
   - `bb logs bramble --since 1h` — historical
   - Output as JSONL events

## DESIGN CONSTRAINTS
- Events are the universal interface — everything emits and consumes them
- Signals are pure functions — no side effects in detection
- Actions are separate from detection — UNIX separation of concerns
- All output is JSONL by default, --human for pretty format

Commit each package separately.
When completely finished, run: openclaw gateway wake --text "Done: Phase 2 event protocol — JSONL events, watcher daemon, signal detection" --mode now
```

---

## Agent 4: Agent Supervisor
**Worktree:** `/tmp/agent-supervisor`

```
You are rewriting the sprite-agent (the on-VM supervisor) as a proper Go daemon.

Read docs/REDESIGN.md for architecture. This binary runs ON the Fly.io sprite VM as the process supervisor for the coding agent (Claude Code / Kimi Code).

## YOUR TASK

1. `internal/agent/supervisor.go` — Process supervisor
   - Launches and monitors the coding agent process (claude/kimi)
   - Handles SIGTERM/SIGINT for graceful shutdown
   - Restarts coding agent on clean exit (iteration loop)
   - Configurable max iterations
   - Timeout handling (max wall-clock time per task)

2. `internal/agent/heartbeat.go` — Heartbeat system
   - Periodic heartbeat goroutine (configurable interval, default 5min)
   - Writes heartbeat event to event log
   - Includes: uptime, iteration count, last commit time, process health

3. `internal/agent/progress.go` — Git-based progress monitoring
   - Background goroutine monitors git activity
   - Tracks: new commits, changed files, branch existence
   - Detects stall: no new commits in N minutes
   - Auto-push: when DONE signal detected, push and create PR

4. `internal/agent/events.go` — JSONL event emission
   - Uses pkg/events types
   - Writes events to: stdout (for log capture) + event log file
   - Events: started, heartbeat, progress, commit, push, done, blocked, error

5. `cmd/bb/agent.go` — The `bb agent` subcommand
   - `bb agent run --task "Fix auth" --repo cerberus --branch fix/auth`
   - Self-contained: reads config from environment or flags
   - Designed to run as PID 1 on a Fly.io VM
   - Clean exit codes: 0=done, 1=error, 130=interrupted

6. Build as static binary for Linux (Fly.io target)
   - `GOOS=linux GOARCH=amd64 go build -o bb-agent ./cmd/bb/`
   - Must work without any runtime dependencies beyond git + coding CLI

## DESIGN CONSTRAINTS
- This is the most complex component — proper process supervision
- Signals (SIGTERM, SIGINT) must be handled correctly
- Goroutines must have proper shutdown (context cancellation)
- Zero external Go dependencies beyond stdlib where possible
- Must be uploadable to a fresh VM and just work

Commit each package separately.
When completely finished, run: openclaw gateway wake --text "Done: Phase 2 agent supervisor — on-sprite daemon with heartbeat, progress, events" --mode now
```
