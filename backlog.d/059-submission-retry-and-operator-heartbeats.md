# Make submission retry and long-run feedback operator-grade

Priority: P1 | Status: ready | Estimate: M

## Goal

Make the submission storm loop recoverable and legible when an operator makes a
pre-execute mistake or a remote verdict run takes minutes.

## Oracle

- [x] A missing declared secret is caught before consuming the canonical
      `storm:<submission>:<kind>` gate key, or the CLI gives a first-class
      retry path that creates a clean replacement submission/member.
- [x] `bb gate --json` explains when a canonical member failed for
      infrastructure/operator reasons and names the next safe command.
- [x] `bb dlq replay` has a JSON output mode, or its help/docs state that it is
      intentionally text-only and must be followed by `bb runs show`.
- [x] Long-running `bb run` invocations expose an early run id and periodic
      status/heartbeat in human mode without corrupting final `--json` output.
- [x] Dogfood docs cover the canonical failure/retry path with a real
      submission example.

## Children

1. Decide whether missing-secret checks belong before run insertion for manual
   dispatch, or whether canonical retry should always open a new submission.
   (done 2026-06-13; settled on explicit clean replacement submission)
2. Add gate/status safe-next-action text for `run:failure` storm members.
   (done 2026-06-13)
3. Add `--json` support or explicit documented text-only behavior for
   `bb dlq replay`. (done 2026-06-13)
4. Add human-mode heartbeat output for long `bb run` waits. (done 2026-06-13)
5. Update the dogfood skill and operator recipes with the settled recovery
   path. (done 2026-06-13)
6. Make safe actions self-contained for agents by including plane context in
   the emitted command. (done 2026-06-13)

## Notes

Why: dogfood of `bearer-auth-8c1be3a` on 2026-06-13 consumed the canonical
verify storm key without `GH_TOKEN`. The pre-execute DLQ replay succeeded, but
`bb gate` correctly ignored the replay because it only honors the canonical
`storm:<submission>:<kind>` run. The operator had to infer that a clean
submission was the only practical recovery path.

Evidence:

- Submission `4f6a9da5b948` escalated because canonical verify run
  `0d5c50785324` failed with `secret env var 'GH_TOKEN' not set`.
- `GH_TOKEN=$(gh auth token) bb dlq replay 6` created successful replay run
  `9b7982da52fa`, but the gate still reported verify as `run:failure`.
- `bb dlq replay 6 --json` failed with `unexpected argument '--json'`.
- Long correctness and simplification runs produced no foreground heartbeat for
  multiple minutes before returning final JSON.

Delivery notes:

- 2026-06-13: `bb dlq replay <id> --json` now returns the replayed run bundle
  with `run`, `attempts`, and `events`, matching `bb run --json` and
  `bb runs show --json`. `tests/dlq_cli.rs` covers a pre-execute DLQ replay
  that preserves `parent_run_id` lineage.
- 2026-06-13: human-mode `bb run` now prints an immediate run receipt and
  periodic stderr heartbeats while the run is pending/running. `bb run --json`
  remains quiet until the final machine-readable run bundle. `tests/run_cli.rs`
  covers both surfaces.
- 2026-06-13: `bb gate --json` now adds `safe_next_command` and
  `safe_next_reason` to failed canonical members. The command points at a
  clean replacement submission, not DLQ replay, so the canonical gate key stays
  rigorous while the operator gets an explicit recovery path.
- 2026-06-13: live dogfood submission `9dd735ad16ce` hit canonical
  `simplification` failure `e9edd6960249` (`harness exit 1: Error:
  connection closed`). `bb gate --json` escalated and included the clean
  replacement command/reason. Replacement submission `02e587ec4533` ran all
  unparked members to pass and stayed pending only because `security` remains
  parked.
- 2026-06-13: `safe_next_command` now includes the loaded plane's
  `--config` path, making the emitted retry command runnable from another cwd.
  Structured argv remains a better long-term schema candidate for backlog 053.
