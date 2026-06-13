# 2026-06-13 Bitterblossom Dogfood Notes

Goal: use Bitterblossom while finding and delivering one new issue.

## Notes

- `bb check` is useful and fast, but it is doing double duty as a task
  inventory view. It validates config and prints loaded tasks, but does not
  show parked state.
- `bb task --help` exposes `park` and `unpark` only. My next instinct was
  `bb task list`, which failed with `unrecognized subcommand 'list'`.
- `/api/tasks` has the right shape, but needing `bb serve` for local task
  inventory is friction when an operator or agent is already in a shell.
- The first `bb task list` implementation proved the value immediately:
  it surfaced that the `security` verdict task is parked with
  `run cost $0.2539 > max_cost_per_run_usd $0.25`.
- After pushing the branch, I tried to use `bb run verify` as a plane-native
  gate receipt. The CLI help accepts arbitrary JSON payload, but the verdict
  task failed before execution because the payload had no `submission` field.
  The run dead-lettered after three pre-execute attempts, and there is no
  obvious "acknowledge this intentional failed probe" command.
- Long model names made fixed-width table output hard to scan. The final
  implementation uses compact JSON-lines in text mode to stay inside the
  5k-LOC spine budget; a richer table belongs in a future cleanup only if
  it can delete or share code.

## Selected Issue

Backlog `045`: add `bb task list` with text and JSON output.

## Desired Future Improvements

- `bb run` could print artifact paths for failures and successes; today I
  have to know to follow with `bb runs show`.
- `bb run <verdict-task>` should either validate the required `submission`
  payload before dispatch or make the verdict-task requirement visible in
  help/errors without spending three attempts.
- Failed manual dogfood probes need a clear operator disposition path:
  replay is not the same as acknowledging a known bad invocation.
- The CLI could expose one canonical "operator snapshot" command that joins
  tasks, recent runs, DLQ, parked state, and gate status.
- Parked verdict tasks need an operational follow-up path: `bb task list`
  reveals the state, but it does not tell me whether unpark is safe.
