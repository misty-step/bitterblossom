# Bitterblossom Factory Lane Dogfood Plan

Status: milestone achieved. Branch: `factory/bitterblossom-lane-20260701`.

## Context

- Goal: prove Bitterblossom can chew one real backlog item through the checked-in `build` task on Sprites, then use the run evidence to harden the agent-first surface.
- Factory bar: Bitterblossom is the keystone because the factory needs off-laptop agent execution. The first proof is backlog item to Sprite to branch/report.
- Product boundary: keep runtime mechanics in Rust; workload judgment stays in task cards and backlog packets.
- Selected first slice: dispatch `backlog.d/078-read-only-mcp-server.md` through `bb run build` as a scoped first MCP slice. MCP is the repeated factory gap and is explicitly prioritized by the lane.
- Outward-facing pause: operator approved `bb run build` and pushing the `bb/build/...` branch on 2026-07-01.

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

## Builder Run Evidence

- Dispatch command: the `bb run build` command above, with `GH_TOKEN=$(gh auth token)` and `--config plane`.
- Run id: `a78a6b73b18f`; trace: `fd59e922e5ff`.
- Task/agent/substrate: `build`, `bb-builder-rust@v2`, Sprites on `misty-step/lane-1`, OMP via OpenRouter `z-ai/glm-5.2`.
- Ledger result: `state=success`, attempt phase `released`, exit code `0`, ended `2026-07-01T16:36:33.55011Z`.
- Cost and usage: `$2.2418345600000014`, `1,079,582` tokens in, `33,077` tokens out, `97` turns.
- Artifact surface: `./target/debug/bb --config plane artifacts list a78a6b73b18f --json` returned `REPORT.json`, `result.md`, `LANE_CARD.md`, `EVENT.json`, `RUN.json`, `stdout.txt`, and empty `stderr.txt`.
- Report read: `./target/debug/bb --config plane artifacts read a78a6b73b18f REPORT.json`.
- Builder report status: `ready`; branch `bb/build/078-read-only-mcp-first-slice`; commit `d0e737729e76b9b921d02f757718753d9e52b49b`.
- Remote branch proof: `git ls-remote --heads origin bb/build/078-read-only-mcp-first-slice` returned `d0e737729e76b9b921d02f757718753d9e52b49b`.
- Draft PR: <https://github.com/misty-step/bitterblossom/pull/870>.

## Builder Output Summary

- Added `bb mcp serve` as a read-only stdio MCP server.
- Exposed `tools/list`, `bb_status`, and `bb_check`.
- Kept MCP as a thin adapter over canonical view helpers: `health::status_view` and a new `serve::check_view`.
- Added subprocess stdio coverage in `tests/mcp_cli.rs`.
- Updated `docs/spine.md` and `skills/bitterblossom/SKILL.md` with the new MCP route.
- Raised the spine LOC tripwire from `6000` to `6300`; builder report argues this is mechanism, not workload judgment.

## Lead Verification

- Local checkout: `git switch --track refs/remotes/origin/bb/build/078-read-only-mcp-first-slice`.
- Whitespace check: `git diff --check refs/remotes/origin/master..HEAD` passed.
- Local gate: `./scripts/verify.sh` passed on the builder branch. The gate included fmt, clippy, the full Rust test suite, the new MCP CLI test, plane config checks, local-plane golden path, operations smoke drill, and LOC tripwire `6199 <= 6300`.
- PR check: `gh pr view 870 --json number,url,state,isDraft,headRefName,baseRefName,title` confirmed open draft PR #870 from `bb/build/078-read-only-mcp-first-slice` to `master`.

## Landmark Integration

- Read Landmark lane contract plus Landmark `VISION.md`, `CHANGELOG.md`, `action.yml`, `README.md`, `docs/agent-integration.md`, and the fleet adoption notes.
- Updated Bitterblossom release intelligence wiring on this branch:
  - `.github/workflows/landfall-release.yml` -> `.github/workflows/landmark-release.yml`
  - `.landfall.yml` -> `.landmark.yml`
  - `misty-step/landfall@90249a8...` -> `misty-step/landmark@v1`
  - added workflow permissions and `node-version: "24"` per current Landmark action surface.
- Verification: `/Users/phaedrus/Development/landmark/target/debug/landmark doctor --repo-root .` passed.
- Workflow parse/lint: `actionlint .github/workflows/landmark-release.yml` passed; Ruby YAML parsing passed for both `.github/workflows/landmark-release.yml` and `.landmark.yml`.

## Verification Plan

1. Done: inspected build run with `bb runs show a78a6b73b18f --json`.
2. Done: read artifacts through public surface, not paths: `bb artifacts list a78a6b73b18f --json` and `bb artifacts read a78a6b73b18f REPORT.json`.
3. Done: fetched the builder branch and ran `./scripts/verify.sh` locally.
4. Done: opened draft PR #870 for reviewable product evidence.
5. Deferred: submission storm/gate remains the next BB-native review step after this milestone report.

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
- `bb run build` successfully moved a real backlog item off the laptop and produced a pushed branch plus `REPORT.json`.
- Artifact APIs were sufficient to inspect the run without spelunking local attempt directories.

### Bad

- Huge default `runs list --json` and `submit list --json` outputs are too large for milestone summaries without custom `jq`.
- The selected Sprite org was wrong for this directory until manually corrected.
- While running, `bb run --json` was silent until final output and the ledger only showed `executing`; there was no mid-run progress, branch, or heartbeat signal in `runs show`.

### Ugly

- The local plane still carries 15 old open DLQs; most are known missing-`GH_TOKEN` storm failures, but they create noise for operator truth.

### Friction

- The lane asks for "chew a backlog" but outward-facing builder dispatch can push a branch. That pause boundary needs to be explicit every time.
- The slash-heavy remote branch shorthand `origin/bb/build/078-read-only-mcp-first-slice` was ambiguous in `git log`; full ref names worked.

### Delight

- The Sprite builder completed the full backlog-to-branch loop in about 22.6 minutes, including verify, report, and push.

## Next Best Action

Review PR #870, then run the BB submission storm/gate on the builder branch if we want the next level of product evidence before merge. In parallel, promote backlog 087-style progress/stale signals because this successful run still exposed weak mid-run observability.
