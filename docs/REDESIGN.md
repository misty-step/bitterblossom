# Bitterblossom Redesign — Ousterhout × UNIX

*"bb is to AI agent fleets what kubectl is to containers."*

---

## Design Philosophy

### Ousterhout Principles (A Philosophy of Software Design)

1. **Deep modules** — Each subcommand has a trivial interface (`bb dispatch bramble "Fix auth"`) hiding enormous complexity (preflight → repo setup → prompt render → agent launch → progress monitoring → auto-push → PR creation). The user says *what*, bb figures out *how*.

2. **Information hiding** — Sprite VM state, git credentials, Fly.io API details, composition resolution — all invisible to the caller. A sprite is an opaque handle.

3. **Define errors out of existence** — Provision is idempotent (provision an existing sprite = no-op with verification). Dispatch with failed preflight = automatic retry after fix. Teardown a nonexistent sprite = success. No state where "it ran but we don't know what happened."

4. **Different layer, different abstraction** — Three clean layers:
   - **CLI layer** (`cmd/bb/`) — flag parsing, output formatting, exit codes
   - **Domain layer** (`internal/`) — business logic, composition resolution, fleet state
   - **Infrastructure layer** (`pkg/fly/`, `pkg/git/`) — Fly.io API, git operations, SSH

5. **Strategic programming** — Invest 10-20% extra now in clean interfaces. Every external dependency goes through an interface. Every state transition is explicit. Tests are first-class.

### UNIX Principles

1. **Do one thing well** — Each subcommand is a focused tool. `bb status` reports. `bb dispatch` assigns. `bb watch` monitors. No god-commands.

2. **Text streams as universal interface** — All output is JSONL by default. Machine-readable. Pipeable. `bb status --json | jq '.sprites[] | select(.state == "idle")'`

3. **Compose small tools** — `bb preflight bramble | bb dispatch bramble` — preflight outputs readiness, dispatch consumes it. Watch outputs events, other tools consume them.

4. **Convention over configuration** — Sensible defaults everywhere. `bb dispatch bramble "Fix auth"` just works — finds the composition, resolves the repo, runs preflight, launches. Zero flags for the common case.

5. **Silence is golden** — Success is quiet. Only errors and requested output to stdout. `--verbose` for humans wanting to watch.

---

## Architecture

```
bb/
├── cmd/bb/                    # CLI entry point (thin)
│   ├── main.go                # cobra root command
│   ├── dispatch.go            # bb dispatch
│   ├── provision.go           # bb provision  
│   ├── teardown.go            # bb teardown
│   ├── status.go              # bb status
│   ├── watch.go               # bb watch (daemon)
│   ├── health.go              # bb health
│   ├── prs.go                 # bb prs
│   ├── sync.go                # bb sync
│   ├── compose.go             # bb compose (reconcile fleet)
│   └── logs.go                # bb logs
├── internal/
│   ├── sprite/                # Core sprite abstraction
│   │   ├── sprite.go          # Sprite type, state machine
│   │   ├── lifecycle.go       # provision → dispatch → teardown
│   │   └── sprite_test.go
│   ├── fleet/                 # Fleet-level operations
│   │   ├── fleet.go           # Fleet state, reconciliation
│   │   ├── composition.go     # YAML composition → desired state
│   │   ├── reconcile.go       # Desired vs actual → actions
│   │   └── fleet_test.go
│   ├── dispatch/              # Task dispatch engine
│   │   ├── dispatch.go        # Task assignment, prompt rendering
│   │   ├── preflight.go       # Pre-dispatch validation
│   │   ├── ralph.go           # Ralph loop management
│   │   └── dispatch_test.go
│   ├── watch/                 # Event-driven fleet management
│   │   ├── watcher.go         # Signal detection, event loop
│   │   ├── signals.go         # DONE, BLOCKED, DEAD, STALE
│   │   ├── actions.go         # Auto-push, alert, redispatch
│   │   └── watch_test.go
│   ├── agent/                 # On-sprite supervisor (runs ON the VM)
│   │   ├── supervisor.go      # Process supervisor, heartbeats
│   │   ├── progress.go        # Git-based progress detection
│   │   ├── events.go          # JSONL event emission
│   │   └── agent_test.go
│   ├── health/                # Health checking
│   │   ├── checker.go         # Deep health: git, process, files
│   │   ├── report.go          # Health report types
│   │   └── health_test.go
│   └── prs/                   # PR lifecycle
│       ├── shepherd.go        # Monitor, triage, act on PRs
│       ├── merge.go           # Safe merge with dry-run
│       └── prs_test.go
├── pkg/                       # Reusable infrastructure
│   ├── fly/                   # Fly.io API client
│   │   ├── client.go          # HTTP client for Fly.io
│   │   ├── machines.go        # Machine CRUD
│   │   └── fly_test.go
│   ├── git/                   # Git operations
│   │   ├── repo.go            # Clone, push, branch, commit
│   │   ├── progress.go        # Activity detection
│   │   └── git_test.go
│   ├── github/                # GitHub API (via gh CLI or API)
│   │   ├── prs.go             # PR operations
│   │   ├── notifications.go   # Notification management
│   │   └── github_test.go
│   └── events/                # JSONL event protocol
│       ├── event.go           # Event types, serialization
│       ├── emitter.go         # Stdout JSONL emitter
│       └── events_test.go
├── compositions/              # Team hypotheses (YAML, unchanged)
├── sprites/                   # Sprite personas (unchanged)
├── base/                      # Base config (unchanged)
└── observations/              # Learning journal (unchanged)
```

---

## Core Abstractions

### Sprite (the deep module)

```go
// Sprite is an opaque handle to a running AI agent on Fly.io.
// It hides: VM state, git credentials, process management, health.
type Sprite struct {
    Name        string
    Persona     Persona
    State       State  // provisioned | idle | working | done | blocked | dead
    Machine     fly.Machine
    Assignment  *Assignment
}

// State machine: only valid transitions allowed.
// provisioned → idle (via bootstrap)
// idle → working (via dispatch)
// working → done | blocked | dead (via signals)
// done → idle (via reassign) or teardown
// Any → teardown
```

### Fleet (reconciliation engine)

```go
// Fleet manages desired state (composition YAML) vs actual state (live VMs).
// Calling Reconcile() produces a list of Actions to converge.
type Fleet struct {
    Desired  Composition  // from YAML
    Actual   []Sprite     // from Fly.io + git
}

func (f *Fleet) Reconcile() []Action {
    // Provisions missing sprites
    // Tears down extra sprites
    // Updates configs on changed sprites
    // Returns actions (dry-run safe)
}
```

### Event Protocol (UNIX composability)

```jsonl
{"ts":"2026-02-07T18:00:00Z","sprite":"bramble","event":"dispatch","task":"Fix auth middleware","repo":"cerberus"}
{"ts":"2026-02-07T18:05:00Z","sprite":"bramble","event":"progress","commits":3,"files_changed":7}
{"ts":"2026-02-07T18:30:00Z","sprite":"bramble","event":"done","branch":"fix/auth-middleware","pr":51}
{"ts":"2026-02-07T18:30:01Z","sprite":"bramble","event":"idle"}
```

Every `bb` subcommand can emit and consume these events. `bb watch` is just an event loop that reads signals and emits actions.

---

## Safety Model

**The Iron Rule: Dry-run by default for all mutating operations.**

```bash
# Shows what WOULD happen (default)
bb dispatch bramble "Fix auth"
# Output: "Would dispatch to bramble: preflight ✓, repo cerberus, branch fix/auth"

# Actually does it
bb dispatch bramble "Fix auth" --execute

# Emergency: skip preflight (requires explicit flag)
bb dispatch bramble "Fix auth" --execute --skip-preflight
```

Mutating operations:
- `bb provision` — creates VMs, uploads credentials
- `bb dispatch` — launches agent processes
- `bb teardown` — destroys VMs
- `bb prs merge` — merges pull requests
- `bb watch --execute` — takes autonomous actions

Read-only operations (no flag needed):
- `bb status`, `bb health`, `bb logs`, `bb prs list`

---

## Key Design Decisions

### 1. Single binary, not a monorepo of scripts
Why: One `go install`, one version, one test suite. Deep modules need to share types and interfaces.

### 2. Fly.io API directly, not shelling out to `fly`/`sprite` CLI
Why: Ousterhout says "define errors out of existence." HTTP API gives us typed errors, retries, timeouts. Shell-outs give us string parsing and silent failures.

### 3. Composition YAML as desired state (kubectl model)
Why: `bb compose apply v2.yaml` reconciles fleet to match. Idempotent. Auditable. Diffable. The YAML IS the truth; the fleet converges to match it.

### 4. On-sprite agent as separate binary (`bb agent`)
Why: The supervisor that runs ON the sprite needs to be a single, self-contained binary with zero dependencies beyond git and the coding CLI. It's uploaded during provision and runs as PID 1 in the VM.

### 5. JSONL event protocol
Why: UNIX composability. `bb watch` emits events, `bb dashboard` consumes them, `bb alert` filters for problems. Pipe-friendly. `bb status --json | jq` for ad-hoc queries.

---

## Migration Path

1. **Phase 1 (current):** Port scripts 1:1 to Go — same behavior, proper tests, --dry-run
2. **Phase 2 (this redesign):** Restructure into deep modules, add Fleet reconciliation, event protocol
3. **Phase 3 (future):** Direct Fly.io API, composition-driven provisioning, experiment framework

Phase 1 gives us safety and tests. Phase 2 gives us architecture. Phase 3 gives us power.

---

## Success Criteria

- [ ] `bb status` shows fleet state in <2 seconds
- [ ] `bb dispatch` runs full preflight + launch in <30 seconds
- [ ] `bb compose apply` reconciles fleet idempotently
- [ ] Zero autonomous actions without `--execute` flag
- [ ] >80% test coverage on all packages
- [ ] `bb agent` binary <10MB, runs on Fly.io VMs
- [ ] All inter-tool communication via JSONL events
- [ ] `go vet && go test ./... && golangci-lint run` clean

---

*"Make each program do one thing well. To do a new job, build afresh rather than complicate old programs by adding new features." — Doug McIlroy*
