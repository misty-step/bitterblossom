# Add a first-class task inventory CLI

Priority: P1
Status: done
Estimate: S

## Goal

Operators and agents can inspect loaded tasks and parked state directly from
the CLI without treating `bb check` as an inventory command or starting
`bb serve` to call `/api/tasks`.

## Oracle

- [x] `bb --config plane task list` prints every loaded task with agent,
      substrate, trigger count, verdict kind, source, and parked state.
- [x] `bb --config plane task list --json` returns machine-readable rows
      with the same data, including `parked`.
- [x] A parked task is visible as parked in both text and JSON output.
- [x] `./scripts/verify.sh` passes.

## Notes

Dogfood failure: while exploring the repo with `bb`, `bb task --help`
showed only `park` and `unpark`, and
`bb --config plane task list` failed with `unrecognized subcommand 'list'`.
The server already exposes the needed shape through `/api/tasks`; the CLI
should expose the same operator fact without requiring a running server.

## Closure 2026-06-13

Closed by `bb task list`, backed by the same task view used by `/api/tasks`.
Text mode emits one compact task object per line; `--json` emits a pretty
array for agents.

Evidence:

- Red test: `cargo test task_list` initially failed because
  `task_list_rows` did not exist.
- Focused test: `cargo test task_list_cli_reports_parked_state` passed and
  exercises the real `bb` binary by parking a task, then reading it through
  `bb task list --json`.
- Live dogfood: `./target/debug/bb --config plane task list --json | jq
  '.[] | select(.task=="security")'` showed `security` parked with
  `run cost $0.2539 > max_cost_per_run_usd $0.25`.
- Repo gate: `./scripts/verify.sh` passed with `src LOC: 5000`.

Shape packet:
[docs/plans/2026-06-13-045-task-list-cli.md](/docs/plans/2026-06-13-045-task-list-cli.md).

Dogfood notes:
[docs/plans/2026-06-13-bb-dogfood-notes.md](/docs/plans/2026-06-13-bb-dogfood-notes.md).
