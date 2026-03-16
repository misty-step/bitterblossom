### Technical Overview: /scripts

`scripts/` is now intentionally small. The supported operator boundary is:

- `cmd/bb/` for sprite transport (`setup`, `dispatch`, `status`, `logs`, `kill`)
- `conductor/` for workflow judgment and durable run state
- `scripts/` only for the remaining prompt/setup helpers that have not been absorbed elsewhere

Current files:

- `builder-prompt-template.md`: the prompt template rendered by `bb dispatch`
- `onboard.sh`: one-time local operator bootstrap
- `lib.sh`: shared shell helpers used by `onboard.sh`
- `sentry-watcher.sh`: standalone Sentry polling utility not replaced by the conductor
- `test_runtime_contract.py`: runtime-model drift guard
- `glance.md`: this orientation doc

Key constraints:

- completion still flows through `TASK_COMPLETE`, `TASK_COMPLETE.md`, and `BLOCKED.md`
- new transport behavior belongs in `cmd/bb`, not in new shell wrappers
- `builder-prompt-template.md` is shared by both `bb` and the Elixir conductor, so path changes require checking both surfaces
