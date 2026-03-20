Blocked on external review infrastructure.

- Branch: `factory/709-1774024209`
- PR: `#759`
- Commit: `1fe29cd1cd334f501dfb103d4685c355ea0a1d99`
- As of 2026-03-20 17:02 UTC, repo-side checks are green (`Elixir Checks`, `Hook Tests`, `Shell Scripts`, `YAML Lint`, `merge-gate`, `trufflehog`).
- External review checks remain in progress after repeated polling: `review / Cerberus · Architecture`, `review / Cerberus · Correctness`, `review / Cerberus · Security`, `review / Cerberus · Testing`, and `CodeRabbit`.
- Latest CodeRabbit refresh picked up the new commit and its earlier test-coverage suggestions are addressed; the remaining blocker is those review jobs not reaching a terminal state.

Work completed before block:

- Added regression coverage for `current_run_worker/1` fallback when `run_status/1` raises.
- Added regression coverage for the defensive path where the run control module does not export `status/1`.
- Ran `cd conductor && mix test test/conductor/orchestrator_test.exs` with `90 tests, 0 failures`.
