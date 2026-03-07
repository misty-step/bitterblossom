# Conductor

The conductor is the workflow brain. It decides when work starts, when it is blocked, when it needs revision, and when it is safe to merge.

File: [`scripts/conductor.py`](../../scripts/conductor.py)

## Module Shape

```mermaid
flowchart TD
    Intake["Intake\nget_issue / list_candidate_issues"] --> Lease["Lease\nacquire_lease / touch_run / release_lease"]
    Lease --> Route["Routing\npick_issue / select_worker"]
    Route --> Build["Builder Dispatch\nrun_builder"]
    Build --> Review["Reviewer Council\nrun_review_round"]
    Review --> Gate["Governance\nCI, threads, trusted external reviews"]
    Gate --> Merge["Merge + Reconcile\nmerge_pr / reconcile_run"]

    DB["SQLite\nruns + leases + reviews + events"] --- Lease
    DB --- Review
    DB --- Merge
    Log["events.jsonl"] --- Review
    Log --- Merge
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

- `select_worker(...)`
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
