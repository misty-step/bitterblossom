# Support indefinite conductor runtime

Priority: high
Status: ready
Estimate: S

## Goal
The conductor should run indefinitely by default, not time out after 60 minutes. The current `session_timeout_minutes` default of 60 forces manual restarts during overnight or 24/7 operation.

## Problem
`Config.session_timeout_minutes/0` defaults to 60. The CLI's `cmd_start` uses this to set a timer that shuts down the conductor. During the overnight factory audit (2026-04-03), the conductor timed out twice, requiring restarts.

## Sequence
- [ ] Change `session_timeout_minutes` default from 60 to `:infinity`
- [ ] Or: add a `--no-timeout` flag to `mix conductor start` that sets `:infinity`
- [ ] Keep the timeout available for CI/test contexts where bounded runtime is useful
- [ ] Test: verify conductor runs past 60 minutes with the new default

## Oracle
- [ ] `mix conductor start` runs indefinitely by default
- [ ] `mix conductor start --timeout 60` still works for bounded runs
- [ ] `mix test` passes
