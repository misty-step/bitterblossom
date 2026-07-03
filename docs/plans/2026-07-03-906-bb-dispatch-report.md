# bitterblossom-906 children 1+2 report

Lane: `codex-bb-dispatch`

Scope shipped:

- `bb dispatch --repo <path> --brief <file> [--model <slug>] [--label <s>]`
- `bb logs -f <run-id>`

Scope intentionally not shipped:

- Child 3: steering an in-flight run.
- Child 4: completion hand-back.

## Implementation

`bb dispatch` is an operator convenience wrapper over the existing manual
ingress path. It picks `BB_DISPATCH_TASK`, then `dispatch`, then `build`, then
a single manual task if the plane is otherwise unambiguous. The brief file is
read at enqueue time and persisted as `bb.dispatch_job.v1` with canonical
`repo`, `prompt`, optional `model`, `label`, and `branch_slug`; the brief path
is not persisted. `--model` overrides the selected task agent model for that
run before the attempt is prepared.

`bb logs -f` follows ledger events plus released `stdout.txt` and `stderr.txt`
artifacts until the run reaches a terminal state.

## Fresh Critic

A fresh artifact-only Pi critic reviewed the diff against the acceptance oracle.
Accepted fixes:

- Removed persisted brief paths and duplicate prompt fields from the payload.
- Made selected task visible on stderr while keeping stdout machine-clean.
- Limited log artifact streaming to stdout/stderr.
- Enforced a 1 MiB brief cap.
- Made `--model` an actual per-run override.
- Warn on artifact list/read errors.

Waived for this child scope:

- A separate `--task` flag would expand the requested command surface; use
  `BB_DISPATCH_TASK` for explicit selection.
- HTTP log transport is not required for this lane; deployed evidence uses the
  installed `bb` binary inside the authenticated Fly machine.
- `logs -f` keeps normal tail-follow semantics with no timeout.

## Verification

Commands:

```bash
cargo test --test run_cli -- --nocapture
cargo test --test cli_contract_docs -- --nocapture
./scripts/verify.sh
```

Result:

```text
==> spine LOC bloat tripwire (<= 10800; mechanism only -- the Python conductor died of bloat)
    src LOC: 10755
==> verify: all gates green
```

## Local Acceptance Demo

Command shape:

```bash
./target/debug/bb --config "$ROOT" dispatch \
  --repo "$PWD" \
  --brief "$ROOT/brief.md" \
  --model openrouter/local-demo \
  --label codex-bb-dispatch-local

BB_LOGS_POLL_MS=100 ./target/debug/bb --config "$ROOT" logs -f "$RUN_ID"
```

Transcript:

```text
LOCAL_RUN_ID=6e7b29172f33
LOCAL_DISPATCH_STDERR=queued run 6e7b29172f33 task=dispatch; follow with `bb logs -f 6e7b29172f33`
2026-07-03T17:25:40.740163Z state:running
2026-07-03T17:25:40.741278Z progress phase:prepared
2026-07-03T17:25:40.741456Z progress phase:executing
2026-07-03T17:25:41.052268Z progress phase:collecting
2026-07-03T17:25:41.052934Z progress phase:finalizing
2026-07-03T17:25:41.053464Z progress phase:released
2026-07-03T17:25:41.053959Z state:success
--- attempt 1 stdout.txt ---
local-dispatch-demo-start
{"branch_slug":"codex-bb-dispatch-local","label":"codex-bb-dispatch-local","model":"openrouter/local-demo","prompt":"Trivial real job: echo this brief through the dispatch task.\n","repo":"/Users/phaedrus/Development/bitterblossom","schema_version":"bb.dispatch_job.v1"}
local-dispatch-demo-done
terminal state=success
LOCAL_ATTEMPT_MODEL=openrouter/local-demo
```

## Deployed Acceptance Demo

Pending after merge and deploy. The deployed demo should run through the
authenticated Fly plane using the installed `/usr/local/bin/bb`, enqueue a
temporary harmless command-harness dispatch task, follow it with `bb logs -f`,
then remove the temporary task files after the terminal state.
