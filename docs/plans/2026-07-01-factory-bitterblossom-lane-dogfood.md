# Bitterblossom Factory Lane Dogfood Plan

Status: milestone 1 plan. Branch: `factory/bitterblossom-lane-20260701`.

## Context

- Goal: prove Bitterblossom can chew one real backlog item through the checked-in `build` task on Sprites, then use the run evidence to harden the agent-first surface.
- Factory bar: Bitterblossom is the keystone because the factory needs off-laptop agent execution. The first proof is backlog item to Sprite to branch/report.
- Product boundary: keep runtime mechanics in Rust; workload judgment stays in task cards and backlog packets.
- Selected first slice: dispatch `backlog.d/078-read-only-mcp-server.md` through `bb run build` as a scoped first MCP slice. MCP is the repeated factory gap and is explicitly prioritized by the lane.
- Outward-facing pause: `bb run build` may push a remote `bb/build/...` branch, so dispatch waits for operator approval after this milestone.

## Preflight Evidence

- Repo branch: `factory/bitterblossom-lane-20260701` from `origin/master`.
- Clean tree at milestone: `git status --short --branch --untracked-files=all`.
- Built branch-local binary: `cargo build --locked` passed.
- Plane config: `./target/debug/bb --config plane check` loaded 30 tasks.
- Plane health summary: cost today `$0.0889416015` of `$25.00`; 0 parked tasks; 0 pending/running/blocked queues; 15 open DLQs.
- Open DLQ pattern: old storm DLQs are mostly missing `GH_TOKEN`; inline `GH_TOKEN=$(gh auth token)` preflight now passes.
- Sprite state: local `sprite` selection was `adminifi`; corrected to `misty-step/lane-1`.
- Sprite proof: `sprite exec -- whoami` returned `sprite`.
- Build preflight: `GH_TOKEN=$(gh auth token) ./target/debug/bb --config plane preflight build --json` returned no findings.
- Storm preflight: `GH_TOKEN=$(gh auth token) ./target/debug/bb --config plane preflight --storm --json` returned no findings.

## Chosen Work Shape

Dispatch one paid builder run:

```bash
GH_TOKEN=$(gh auth token) ./target/debug/bb --config plane run build \
  --payload '{"repo":"misty-step/bitterblossom","backlog":"backlog.d/078-read-only-mcp-server.md","branch_slug":"078-read-only-mcp-first-slice","dry_run":false,"packet":"Implement the narrow first slice of backlog 078. Add a read-only MCP stdio entrypoint only if it can stay a thin adapter over existing CLI/API view helpers. Prioritize bb mcp serve plus tools/list and one or two low-risk read tools such as bb_status and bb_check. Do not add mutating MCP tools. Do not move workload judgment into Rust. If the shared view helper extraction is larger than expected, stop after the minimal helper seam plus tests and document the remaining tool list in REPORT.json. Run ./scripts/verify.sh, commit, push branch bb/build/078-read-only-mcp-first-slice, and write REPORT.json with commands, run ids, cost, and residual risk."}' \
  --json
```

## Verification Plan

1. Inspect build run with `bb runs show <id> --json`.
2. Read artifacts through public surface, not paths: `bb artifacts list <id> --json` and `bb artifacts read <id> REPORT.json`.
3. Fetch the builder branch and run `./scripts/verify.sh` locally.
4. If the branch is reviewable, open/update a draft PR only after reporting that outward-facing step.
5. Submit and storm using the checked-in recipe or explicit `bb submit` flow with `GH_TOKEN=$(gh auth token)`, then evaluate `bb gate --submission <id> --json`.

## Stop Conditions

- If `bb run build` fails or blocks, do not quietly implement locally. Record the blocker as dogfood evidence and stop.
- If a run reaches or passes `executing`, do not replay mechanically.
- If preflight starts failing again, stop before paid execution.
- If the builder creates a branch but no `REPORT.json`, treat that as a product finding.
- If the implementation attempts workload-specific spine logic, reject or reshape the work.

## Backlog Fit

- Primary: `078` read-only MCP server.
- Supporting evidence from lane: `083` guardrails and `086` recipes remain prerequisites for unattended volume, but the first manually approved builder run is currently safe because preflight passes and the plane has no active queue.
- Cross-lane note: Canary integration stays report-only until `080` evidence and authority scorecards are green.

## UX Notes So Far

### Good

- `bb status --json` gave a compact operator health summary and safe-next-action hints.
- `bb preflight` correctly separated missing env binding from actual credential availability once `GH_TOKEN=$(gh auth token)` was used.

### Bad

- Huge default `runs list --json` and `submit list --json` outputs are too large for milestone summaries without custom `jq`.
- The selected Sprite org was wrong for this directory until manually corrected.

### Ugly

- The local plane still carries 15 old open DLQs; most are known missing-`GH_TOKEN` storm failures, but they create noise for operator truth.

### Friction

- The lane asks for "chew a backlog" but outward-facing builder dispatch can push a branch. That pause boundary needs to be explicit every time.

## Next Best Action

After operator approval, run the exact `bb run build` command above and stop again when the builder run reaches a terminal state or produces a branch/report.
