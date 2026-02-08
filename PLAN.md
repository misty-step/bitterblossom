# Issue #29 Plan

## Execution

- [x] Add PTY-flush logging path in `scripts/sprite-agent.sh` with safe fallback.
- [x] Update Go dispatch launch script in `internal/dispatch/dispatch.go` to prefer PTY-flush logging.
- [x] Extend output classification in `internal/agent/progress.go` for tool/file/command/error snippets.
- [x] Improve human-readable log rendering in `cmd/bb/logs.go` for progress snippets.
- [x] Add/adjust tests in `internal/dispatch/dispatch_test.go`, `internal/agent/progress_test.go`, `cmd/bb/logs_extra_test.go`.
- [x] Add `--follow` to `scripts/tail-logs.sh` for live remote tails.
- [x] Run validation (`go test ./...`, `shellcheck scripts/*.sh`) and fix regressions.

## Review

- Real-time path now prefers PTY-backed `script -qefc` with fallback when unavailable.
- Progress events now emit `tool_call`, `file_edit`, `command_run`, and existing `error`.
- Human `bb logs` output now includes progress activity/detail snippets.
- Validation:
  - `go test ./...`
  - `make test`
  - `make build`
  - `find scripts -type f -name '*.sh' -print0 | xargs -0 shellcheck -x -S error`
  - `python3 -m pytest -q base/hooks`
