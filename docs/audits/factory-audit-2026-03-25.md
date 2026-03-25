# Factory Audit Report

## Summary

- Date: March 25, 2026
- Run ID: `run-794-1774451703`
- Issue: [#794](https://github.com/misty-step/bitterblossom/issues/794) `[retro] Workspace preparation bash syntax error blocks all work`
- PR: [#795](https://github.com/misty-step/bitterblossom/pull/795) `fix(conductor): validate workspace bash scripts`
- Worker: `bb-builder`
- Reviewers: `bb-fixer`, `bb-polisher`
- Terminal State: `pr_opened` with CI green, `CodeRabbit` pending, Thorn idle, and Fern dispatch observed but not completed before shutdown

## Timeline

| Time (UTC) | Event | Notes |
|------|-------|-------|
| 2026-03-25 15:08 | audit preflight | Confirmed local auth resolution was `{:chatgpt, "/Users/phaedrus/.codex/auth.json"}` and remote `bb-builder` had `auth_file=yes`, `api_env=no` |
| 2026-03-25 15:09 | first scoped run of `#792` | Failed in workspace preparation with `bash: -c: line 19: syntax error near unexpected token \`|'` |
| 2026-03-25 15:10 to 15:12 | local branch repair | Rewrote stale-worktree cleanup pipeline, added shell-parse tests, and verified `mix test test/conductor/workspace_test.exs test/conductor/run_server_test.exs` |
| 2026-03-25 15:15 | live run start | Conductor shaped `#794`, started `run-794-1774451703`, prepared workspace, and dispatched Weaver on `bb-builder` |
| 2026-03-25 15:15 | live auth proof | Active `codex` process environment on `bb-builder` contained `HOME` and `EXA_API_KEY`, but no `OPENAI_API_KEY` or `CODEX_API_KEY` |
| 2026-03-25 15:18 | PR opened | Weaver committed `4d4642c`, pushed `factory/794-1774451703`, and created PR `#795` |
| 2026-03-25 15:19 | CI green | `Shell Scripts`, `Hook Tests`, `YAML Lint`, `Elixir Checks`, `merge-gate`, and `trufflehog` all passed |
| 2026-03-25 15:22 | conductor state truthfully advanced | Run moved to `pr_opened` with PR `#795` and workspace cleanup verified |
| 2026-03-25 15:24 to 15:26 | review window | `CodeRabbit` remained `pending`; Gemini review reported no findings |
| 2026-03-25 15:25 | Fern dispatch | Conductor logged `[fern] PR #795 is green, dispatching Fern`; no public Fern output landed before shutdown |

## Findings

### Finding: OAuth auth path is working live

- Severity: resolved
- Existing issue or new issue: existing issue [#792](https://github.com/misty-step/bitterblossom/issues/792) validated after prior branch fixes
- Observed: The live builder workspace used `/home/sprite/.codex/auth.json`, `.bb-runtime-env` contained only `EXA_API_KEY`, and the active `codex` processes had no `OPENAI_API_KEY` or `CODEX_API_KEY` in `/proc/<pid>/environ`.
- Expected: Managed Codex sprites should authenticate with the synced ChatGPT/Pro account cache by default and omit API-key env injection.
- Why it matters: This was the core purpose of `cx/codex-pro-auth-default`; without live proof the branch would still be speculative.
- Evidence: `Conductor.Config.codex_auth_source() -> {:chatgpt, "/Users/phaedrus/.codex/auth.json"}` locally; remote `auth_file=yes`, `api_env=no`; live PIDs `4576` and `5049` showed `HOME` and `EXA_API_KEY`, but no OpenAI API env vars.

### Finding: Workspace-preparation regression was real and branch-local

- Severity: critical, fixed during audit
- Existing issue or new issue: existing retro issue [#794](https://github.com/misty-step/bitterblossom/issues/794)
- Observed: The first live runs on `#792` failed immediately during workspace setup because the generated shell script emitted a newline before a follow-on pipe: `done` on one line, `| while ...` on the next.
- Expected: Worktree cleanup snippets must be bash-parseable before dispatch, especially on the factory critical path.
- Why it matters: This blocked every Weaver run before any useful work could begin.
- Evidence: `workspace_preparation_failed` at `2026-03-25T15:09:03Z` and `2026-03-25T15:11:21Z`; after the fix, `run-794-1774451703` prepared its workspace successfully and completed cleanup verification.

### Finding: Weaver can complete a live issue-to-PR path again, but the handoff is still sticky

- Severity: medium
- Existing issue or new issue: no new backlog issue created during this audit
- Observed: Weaver completed a real fix on `#794`, ran targeted tests, pushed a branch, and opened PR `#795`. However, it then waited on review/CI surfaces inside the builder session for several minutes before exiting, so the run stayed in `building` until `15:22:29Z`. Thorn was correctly idle because CI never failed. Fern did dispatch once the PR was green, but that happened only after the delayed handoff and did not produce a visible review artifact before shutdown.
- Expected: Weaver should hand control back promptly after PR creation so the conductor, Thorn, and Fern own the follow-on governance loop without builder-side waiting.
- Why it matters: Long builder post-PR waits blur authority boundaries and slow the intended Weaver -> Thorn -> Fern pipeline.
- Evidence: Builder log showed `sleep 130`, followed by `gh pr checks 795` and `gh pr view 795 --comments`; conductor did not move the run to `pr_opened` until after Weaver exited.

## Backlog Actions

- New issues: none during this audit; the blocking regression was already captured as [#794](https://github.com/misty-step/bitterblossom/issues/794)
- Existing issues commented: `#794` was shaped and exercised by the live run; `#792` was removed from the temporary audit label after cooldown made it a poor audit candidate
- Priority changes: none

## Reflection

- What Bitterblossom did well: Once the workspace regression was fixed, the branch authenticated with the ChatGPT/Codex account cache exactly as intended, prepared an isolated workspace, dispatched Weaver, produced a focused patch, and opened a valid PR with green CI.
- What felt brittle: The builder still spent minutes waiting on post-PR review surfaces itself, which delayed conductor truth updates and pushed Fern dispatch later than it needed to be.
- What should be simpler next time: Weaver should return immediately after PR creation and basic validation, leaving CI failure recovery and PR-polish waits entirely to Thorn, Fern, and the conductor loop.
