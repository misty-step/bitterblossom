# Conductor

The conductor is the workflow brain. It decides when work starts, when it is blocked, when it needs revision, and when it is safe to merge.

> **Status:** This doc describes the **Elixir conductor** (`conductor/`), which is the current primary implementation per [ADR-004](../adr/004-elixir-conductor-architecture.md). The Python conductor (`scripts/conductorlib/`) is deprecated.

Current implementation: [`conductor/lib/conductor/`](../../conductor/lib/conductor/)

Key modules:

- `orchestrator.ex` — polling loop, issue selection, run dispatch
- `run_server.ex` — per-run GenServer state machine
- `store.ex` — SQLite persistence (runs, leases, events)
- `github.ex` — GitHub operations via `gh` CLI
- `sprite.ex` — sprite operations via `bb` CLI
- `workspace.ex` — worktree lifecycle
- `shell.ex` — subprocess execution with timeout

## Module Shape

```mermaid
flowchart TD
    CLI["conductor.py\nCLI + orchestrator"] --> Tracker["conductorlib.tracker\nGitHub issue/PR boundary"]
    CLI --> Workspace["conductorlib.workspace\nrun worktree boundary"]
    CLI --> Governance["conductorlib.governance\nmerge/readiness boundary"]
    CLI --> State["in-file state + lease helpers\npending later split"]

    State --- DB["SQLite\nruns + leases + reviews + events"]
    State --- Log["events.jsonl"]
    Tracker --> GH["GitHub"]
```

## Run State

```mermaid
stateDiagram-v2
    [*] --> queued
    queued --> leased
    leased --> building
    building --> reviewing
    reviewing --> revising: review fixes requested
    revising --> reviewing: builder revised
    reviewing --> ci_wait: council quorum
    ci_wait --> revising: CI failure or PR feedback
    ci_wait --> blocked: unresolved trusted feedback / timeout
    ci_wait --> merge_ready: checks settled
    merge_ready --> merged
    building --> failed
    reviewing --> failed
    revising --> failed
    merge_ready --> failed
    blocked --> [*]
    failed --> [*]
    merged --> [*]
```

## Governance Loop

```mermaid
flowchart LR
    PR["PR ready"] --> Council["internal council quorum"]
    Council --> CI["required CI green"]
    CI --> Threads["all conversations resolved"]
    Threads --> External["trusted external reviews settled"]
    External --> Quiet["quiet window elapsed"]
    Quiet --> Merge["squash merge"]

    Council -. fail .-> Revise["builder revision"]
    CI -. fail .-> Revise
    Threads -. fail .-> Revise
    External -. timeout .-> Block["block run"]
```

## Worker Readiness

Recent failure mode: reviewer dispatch failed after the builder already opened a PR because a reviewer sprite had a broken repo checkout.

Current mitigation:

```mermaid
flowchart TD
    Probe["bb dispatch --dry-run"] --> Ok{"ready?"}
    Ok -- yes --> Use["use sprite"]
    Ok -- no --> Repair["bb setup <sprite> --repo <repo> --force"]
    Repair --> Reprobe["probe again"]
    Reprobe --> Ready{"ready now?"}
    Ready -- yes --> Use
    Ready -- no --> Fail["fail run before builder work"]
```

This is a point hardening step, not the final worker-pool design. The longer-term pool manager still belongs in the broader worker-health backlog.

## Persistent Truth

The conductor writes truth in two places:

- `.bb/conductor.db`
- `.bb/events.jsonl`

Use them for:

- current phase and status
- lease ownership and heartbeat expiry
- reviewer verdicts
- append-only event history

GitHub is still the operator-facing conversation surface, but the run store is where the machine remembers what actually happened.

## Key Interfaces

### Intake

- `get_issue(...)`
- `list_candidate_issues(...)`
- `pick_issue(...)`

### State

- `open_db(...)`
- `create_run(...)`
- `update_run(...)`
- `record_event(...)`
- `touch_run(...)`

### Runtime

- `select_worker_slot(...)`
- `run_builder(...)`
- `run_review_round(...)`

### Governance

- `wait_for_pr_checks(...)`
- `list_unresolved_review_threads(...)`
- `wait_for_external_reviews(...)`
- `merge_pr(...)`

## What This Module Should Not Become

- not a second `bb`
- not a generic fleet manager
- not a bag of shell heuristics with implied state
- not a peer-to-peer sprite chat layer

It should stay deep: small operator surface, rich internal orchestration.
