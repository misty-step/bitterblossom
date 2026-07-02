# Coding-Harness Workload and Scoring Rubric

Date: 2026-07-02
Backlog: 058 child 3

## Purpose

This memo defines the repeatable workload that durable-workflow and substrate
candidates must run during the 058 re-check. It is designed to score the exact
work Bitterblossom exists to run: a coding harness operating on a repo
workspace, not a generic function invocation.

No candidate is scored in this slice.

## Workload: `bb-coding-harness-probe.v1`

The candidate executes one hermetic coding-harness run against a real git repo
workspace and returns a structured run bundle.

### Input

- Repo: `misty-step/bitterblossom`
- Ref: caller-provided commit SHA
- Workload card: stdin payload containing:
  - target file path under a temporary candidate workspace;
  - expected artifact paths;
  - one secret value that must not appear in argv, logs, artifacts, branch names,
    or process listings;
  - timeout seconds;
  - optional failure mode: `happy`, `timeout`, `missing_artifact`,
    `stderr_noise`, or `crash_after_progress`.

### Required Execution Steps

1. Prepare a clean workspace at the requested ref.
2. Run a command-shaped harness from that workspace with prompts and secrets
   delivered on stdin or an equivalent non-argv secret channel.
3. Stream stdout and stderr while the command is still running.
4. Emit at least two progress records before completion.
5. Write a required `REPORT.json` artifact and one nested text artifact on the
   happy path.
6. Record provider/model/cost fields when the harness reports usage; record
   explicit zero or unavailable values for command-only probes.
7. Enforce the timeout by killing the executing process or remote session.
8. Preserve enough process/workspace evidence for recovery classification if
   the host, process, marker, or artifact collection becomes uncertain.
9. Return an agent-facing run bundle with state, phase history, artifacts,
   cost, timing, progress, and safe operator action.

### Required Failure Probes

| Probe | Expected result |
|---|---|
| `happy` | Run succeeds, required artifacts are readable, progress is visible before completion, secret is absent from argv/logs/artifacts. |
| `timeout` | Candidate kills the executing harness, records timeout evidence, and returns a terminal failure or explicit recovery state. |
| `missing_artifact` | Zero-exit harness without required artifact is failed, not treated as success. |
| `stderr_noise` | Noisy stderr is retained without corrupting structured result parsing. |
| `crash_after_progress` | Recovery surface can distinguish alive, dead, or unknown and names the safe next operator action. |

## Scoring Rubric

Total: 100 points. A candidate that scores below 70 cannot replace the current
baseline. A candidate below 90 can still justify a narrow borrowed primitive if
the weak areas sit outside the proposed boundary.

| Dimension | Points | Scoring criteria |
|---|---:|---|
| Workspace preparation | 10 | Fresh checkout at exact ref, no stale files, repo size limits named, workspace cleanup or retention policy explicit. |
| Cold start and max runtime | 10 | Cold start measured from accepted run to first harness byte; timeout and max runtime enforced without leaked child processes. |
| Streaming and interactivity | 10 | stdout/stderr available before completion; progress records are machine-readable; long silence can classify as stale. |
| Secret handling | 12 | Prompts/secrets are not passed through argv, branch names, image layers, public logs, or artifacts; inherited env is scrubbed. |
| Artifact extraction | 10 | Required artifacts are collected, safe path checks reject traversal/symlinks/oversized/binary reads, nested artifacts are addressable. |
| Cost and budget control | 10 | Usage is captured per attempt; in-flight cap breach can kill execution; over-budget accepted work is visible as blocked or failed evidence. |
| Concurrency and leases | 10 | Shared host/workspace mutual exclusion holds across concurrent deliveries and process crashes; per-task FIFO is preserved. |
| Failure recovery | 14 | Pre-execute failures are replayable; executing failures are not auto-replayed; alive/dead/unknown recovery has evidence and operator action. |
| Network and egress control | 6 | Network assumptions are explicit; egress can be disabled, allowed, or audited according to task policy. |
| Operator CLI ergonomics | 8 | Candidate evidence projects into stable CLI/API JSON without requiring a vendor console; commands are repeatable locally and in CI. |

## Minimum Passing Contract

A candidate must pass all of these regardless of point score:

- no secret in argv or public logs;
- no success when `REPORT.json` is missing;
- timeout can kill the executing harness;
- executing-phase uncertainty does not auto-replay;
- host/workspace concurrency cannot overlap side-effecting runs;
- the operator can inspect state from `bb` without opening a vendor dashboard.

## Candidate Evidence Packet

Every candidate probe should produce one packet containing:

- candidate name and version/date;
- exact command or API used to launch the workload;
- accepted run id and commit SHA;
- first-byte latency, total duration, and timeout setting;
- streamed stdout/stderr excerpt with secret redaction proof;
- artifact list plus `REPORT.json` content;
- cost/tokens or explicit unavailable reason;
- recovery/failure probe result;
- final score table with residual risks.

## Next Slice Input

The next 058 slice can now prototype only the smallest adapter probes needed to
score candidates against this workload. No product should be adopted until a
candidate evidence packet exists.
