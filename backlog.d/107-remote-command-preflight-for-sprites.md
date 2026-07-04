# Preflight command-harness binaries on the sprite host, not only the local plane host

Priority: P1 | Status: ready | Estimate: M

## Goal

`bb preflight <task>` should catch command-harness binaries that are missing on
the declared sprite host before a production run is created.

## Evidence

Live drill on 2026-07-02:

- `BB_PLANE_DIR=/app/plane bb preflight incident-triage --json` on production
  returned `findings: []`.
- The next manual production run, `1141ed106fea`, prepared the sprite workspace
  and then failed at execute with `harness exit 127`.
- Sprite inspection showed `bitterblossom/scripts/incident-triage-wrapper.sh`
  existed and was executable, but `pi`, `omp`, and `opencode` were not present
  on `misty-step/bb-tansy`.

The incident wrapper now falls back to a pinned `npx` package for `pi`, but the
generic preflight gap remains for future command workloads.

## Oracle

- [x] For `harness = "command"` on `substrate = "sprites"`, preflight checks
      the command on the declared sprite host with the same workspace/path
      semantics used at dispatch time.
- [x] The finding names the missing binary, the host, and the exact task.
- [x] The check stays read-only and does not create a run row.
- [x] A fake-sprite test covers missing remote command and successful remote
      command.
- [x] `./scripts/verify.sh` passes.

## Non-goals

Do not make preflight install dependencies or mutate sprite state. It should
report readiness; wrappers or task specs decide how to remediate.

## 2026-07-04 Slice

Added a sprite-host command-harness probe for `bb preflight <task> --json`.
For `substrate = "sprites"` and `harness = "command"`, preflight now runs a
read-only `sprite exec ... -- sh -c` check against the task's declared host.
Bare command names resolve through the remote PATH; path-like bins are checked
from `/home/sprite/bb/<task>` if that workspace already exists. The
`unspawnable_binary` finding now carries the missing binary, sprite host,
substrate, harness, model, and exact task, and the fake-sprite tests prove both
missing and present remote commands without creating a ledger row.
