## Blocker

The code-side fix is complete, but the live fleet reprovisioning steps are blocked by missing sprite auth in this workspace.

## Verified

- `mix test test/conductor/config_test.exs test/conductor/cli_fleet_test.exs test/conductor/config_dispatch_env_test.exs test/conductor/fleet/reconciler_test.exs test/conductor/sprite_dispatch_test.exs`
- `printf 'Say hello\n' | codex exec --yolo --json --model gpt-5.4 -c web_search=live`
  - succeeded, so the current `OPENAI_API_KEY` in this shell can authenticate with OpenAI

## Still Blocked

- `.env.bb` is absent in this worktree, so I could not rotate or confirm the repo-local runtime config file named in the issue
- `mix conductor fleet --reconcile` fails preflight with:
  - `missing: SPRITE_TOKEN, FLY_API_TOKEN, or sprite CLI auth`
- Without sprite auth, I cannot re-provision remote sprites or verify a real builder dispatch end to end

## Next Operator Action

Provide valid sprite auth via `.env.bb`, `SPRITE_TOKEN`, `FLY_API_TOKEN`, or a logged-in `sprite` CLI session, then rerun:

```bash
cd conductor
GITHUB_TOKEN="$(gh auth token)" mix conductor fleet --reconcile
```
