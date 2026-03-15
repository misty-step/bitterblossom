# Factory Audit Report

## Summary

- Date: 2026-03-15
- Run ID: run-648-1773580938
- Issue: #648 — Governance: rebase on merge conflict instead of full rebuild
- PR: pending (builder completed, blocked on auth)
- Worker: bb-builder
- Reviewers: none (run-once mode, no council)
- Terminal State: **blocked** — GH_TOKEN not injected into agent environment

## Timeline

| Time (UTC) | Event | Notes |
|------------|-------|-------|
| 13:17:56 | first run attempt | failed: workspace not found on sprite |
| 13:18:39 | second run attempt | failed: "Not logged in" — no ANTHROPIC_API_KEY |
| 13:21:59 | fleet re-provisioned | API key pushed to all 3 sprites |
| 13:22:17 | third run attempt | lease acquired, workspace prepared |
| 13:22:19 | builder dispatched | claude-sonnet-4-6 on bb-builder |
| 13:42:27 | builder complete | status=blocked, 20 min build time |
| 13:42:27 | run_blocked event | GH_TOKEN not available in agent env |
| 13:42:28 | retro analysis | automatic post-run analysis |
| 13:44:xx | manual branch push | factory/648-1773580938 pushed to origin |
| 13:45:xx | GH_TOKEN fix applied | sprite.ex: inject Config.dispatch_env() |

## Findings

### Finding 1: Conductor escript NIF path broken

- Severity: P2
- Existing issue: none identified
- Observed: `conductor/conductor` escript fails with exqlite NIF load error — `errno=20` (ENOTDIR) on path resolution
- Expected: escript runs standalone without `mix`
- Why it matters: operators can't use the shipped binary; must use `mix conductor` from the conductor directory
- Evidence: `UndefinedFunctionError: function Exqlite.Sqlite3NIF.open/2 is undefined`

### Finding 2: Fleet "reachable" doesn't verify workspace

- Severity: P2
- Existing issue: none identified
- Observed: `Sprite.reachable?/1` only checks `echo ok` — reports ok even when no repo workspace exists
- Expected: dispatch readiness check should verify the target repo is cloned
- Why it matters: first run failed silently at workspace preparation; operator had to diagnose and run `bb setup` manually
- Evidence: run-648-1773580677 failed with "No such file or directory" for workspace path

### Finding 3: ANTHROPIC_API_KEY not in .env.bb

- Severity: P1 (was blocker, now resolved by operator)
- Existing issue: #658 (Codex agent via OAuth token) is related
- Observed: `.env.bb` had no `ANTHROPIC_API_KEY`; fleet provisioning silently pushed empty key
- Expected: `Fleet.provision!` should fail loudly when API key is empty
- Why it matters: sprite reports reachable but can't run Claude Code; failure surfaces only at dispatch time
- Evidence: `key_length: 0` after provisioning; builder error "Not logged in"

### Finding 4: Go/Elixir auth gap

- Severity: P2
- Existing issue: #624 (persist gh auth on sprites)
- Observed: `bb setup` (Go) configures git credential helper but not Claude Code API key; `Fleet.provision!` (Elixir) pushes settings.json with API key but doesn't configure git auth
- Expected: one setup path that configures both
- Why it matters: operators must run both `bb setup` AND `Fleet.provision!` to get a working sprite

### Finding 5: GH_TOKEN not injected into agent environment

- Severity: P1 (fixed in this audit)
- Existing issue: #624 is the closest match
- Observed: `Sprite.agent_command/3` set `LEFTHOOK=0` but not `GITHUB_TOKEN`; `Config.dispatch_env/0` exists with the right data but was never called
- Expected: dispatch injects all env vars from `Config.dispatch_env()` into the agent command
- Why it matters: builder completed 20 minutes of work but couldn't push or create PR — entire run wasted
- Evidence: builder artifact: "GH_TOKEN not available in agent environment"
- Fix: one-line change in `sprite.ex` to call `Config.dispatch_env()` in `agent_command/3`

### Finding 6: Stale branches accumulate on sprites

- Severity: P3
- Existing issue: none identified
- Observed: workspace cleanup removes worktrees but leaves branches behind (`factory/648-1773580720`, `factory/648-1773580938`)
- Expected: cleanup should prune branches from failed/blocked runs
- Why it matters: branches accumulate over time; git operations slow down

## Backlog Actions

- Finding 1: new issue candidate (P2, escript NIF fix)
- Finding 2: new issue candidate (P2, dispatch readiness probe)
- Finding 3: comment on #658 with evidence
- Finding 4: comment on #624 with evidence
- Finding 5: **fixed in this audit** — committed to `fix/inject-gh-token-into-dispatch`
- Finding 6: new issue candidate (P3, branch cleanup)

## Reflection

- What Bitterblossom did well:
  - Builder produced high-quality code: 409 LOC, 13 new tests, security input validation, clean architecture
  - Builder accurately diagnosed the auth blocker and wrote actionable recovery instructions in the artifact
  - Retro analysis ran automatically post-run
  - Event store captured clean timeline for debugging

- What felt brittle:
  - Three failed attempts before a successful dispatch (workspace missing, API key missing, then auth gap)
  - Zero operator visibility during the 20-minute build — no streaming, no progress events
  - `Config.dispatch_env()` existed but was dead code — easy to miss in review

- What should be simpler next time:
  - `bb setup` + `Fleet.provision!` should be one operation
  - Preflight should verify: sprite reachable, workspace exists, API key non-empty, GH_TOKEN injectable
  - Builder timeout (25 min) was not enforced — builder ran 20 min but the conductor waited without complaint even after timeout would have fired
