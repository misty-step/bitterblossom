---
name: bb-qa
description: |
  QA Bitterblossom changes by exercising the real event plane, not just tests.
  Bitterblossom is one Rust binary (`bb`) with two personalities — `bb serve`
  (webhook/cron ingress + queue + dispatch, read API on :7077) and `bb <verb>`
  (operator/agent CLI) — dispatching agent workloads to a remote substrate
  (Fly Sprites). "Tests pass" is not QA. Use when: "QA this", "verify the
  feature", "smoke test bitterblossom", "check the plane", "test bb".
  Trigger: bb-qa.
argument-hint: "[surface|verb|task|route|feature]"
---

# bb-qa

QA in Bitterblossom means verifying the surface that changed against a real run.
`./scripts/verify.sh` is the deterministic gate (fmt, clippy, tests, model-catalog
fixture, `bb check` on all three planes, the zero-credential local-plane smoke,
`production-ops-drill --local`, and the `src/` LOC tripwire). It is **necessary
but not sufficient**: it runs against *stub harnesses and the local substrate*,
so it proves the plane machinery (ingress, ledger, dispatch, budget, gate
arithmetic) — NOT that a real Sprite executed, a real model ran, or the
externally visible side effect (a PR comment) actually happened. Substrate,
harness, and workload changes need **live evidence** on top of a green gate.

## Surfaces

| Changed area | Surface | QA path |
|---|---|---|
| `src/spec.rs`, `examples/*`, `plane/`, `tasks/`, `agents/` | Config surface | `bb check` on all three planes (verify.sh does this) — read the validation, not just exit 0 |
| `src/ledger.rs`, `dispatch.rs`, `budget.rs`, `recovery.rs`, `submit.rs` | Run machinery / merge gate | `./scripts/verify.sh` + `production-ops-drill --local`; then a live dispatch for real behavior |
| `src/ingress.rs`, `serve.rs`, `notify.rs` | Plane serve / read API / webhook | `./scripts/control-loop-drill.sh` (read API + HTML + signed-webhook containment) |
| `src/substrate/**`, `harness.rs` | Remote execution (Fly Sprites) | Live reflex/dispatch run — verify the ledger row AND the external effect; the gate CANNOT prove this |

## Start local runtime

```sh
cargo build                     # → ./target/debug/bb
./scripts/verify.sh             # the deterministic gate (run first)
```

Three planes (`--config <dir>`):
- `examples/local-plane` — zero-credential, `dev = true`, local substrate; the golden smoke path.
- `examples/demo-plane` — production-shaped, dispatches to remote Sprites over WebSocket.
- `plane/` — this machine's real plane. `bb --config plane serve` binds the read API on **127.0.0.1:7077** (`plane/plane.toml`); the Fly deployment binds `0.0.0.0:8080` behind https (`fly.toml`, app `bitterblossom-plane`).

- Env: `BB_API_TOKEN` gates the read API (loopback is allowed with no token; when the token is set, `Authorization: Bearer $BB_API_TOKEN` is required). Live remote runs need `OPENROUTER_API_KEY`, `GH_TOKEN=$(gh auth token)`, and `SPRITE_TOKEN` — `source .env.bb` loads the Fly/Sprite/OpenRouter credentials.
- Sprite org: **must be `misty-step`** before any dispatch. `.sprite` pins org `misty-step` / sprite `lane-1`; `sprite org list` can drift — fix with `sprite use -o misty-step lane-1` and verify `sprite exec -- whoami`.

## CLI QA — zero-credential golden path (offline, deterministic)

```sh
BB=./target/debug/bb; CFG=examples/local-plane
$BB --config $CFG check --json
$BB --config $CFG preflight hello --json
$BB --config $CFG run hello --payload '{"ok":true}' --json      # note the run id
$BB --config $CFG status --json
$BB --config $CFG runs show <run-id> --json                     # state, cost, attempts, events
$BB --config $CFG artifacts read <run-id> REPORT.json --json    # inspect the side effect, not just exit 0
```

Edge the gate already asserts (re-check if you touched ingest/validation): an
invalid payload (`--payload 'not json'`) must exit non-zero and create **no** run
row — the ledger stays untouched.

## Plane / read-API QA

```sh
./scripts/control-loop-drill.sh    # loopback read API + HTML no-token; bearer-only with BB_API_TOKEN; signed-webhook burst vs max_runs_per_day containment
```

Manual: `bb --config plane serve` (:7077), then curl `/` (HTML), `/api/runs`,
`/api/status`, `/api/dlq`, `/api/tasks`, `/api/gate`, `/api/runs/<id>` with and
without `Authorization: Bearer $BB_API_TOKEN`. (Long-lived server; don't leave it up.)

## Remote / live-run QA (the part the gate can't fake)

For any `substrate/`, `harness.rs`, or workload change. Read-only enumerate first:
`bb --config plane task list --json` for real task names (`review`, `build`,
`verify`, `correctness`, `security`, `simplification`, `product`, `arbiter`, …).

```sh
source .env.bb                              # OPENROUTER_API_KEY, SPRITE_TOKEN, FLY_API_TOKEN
export GH_TOKEN=$(gh auth token)
sprite use -o misty-step lane-1 && sprite exec -- whoami   # org MUST be misty-step
./target/debug/bb --config plane run review --payload '{"repo":"owner/repo","pr":N}' --json
./target/debug/bb --config plane runs show <id> --json     # state, cost, tokens
```

Evidence = the ledger row (`runs show`) **AND** the external effect (the PR
comment). For the submission-loop merge gate (`submit.rs`), drive `bb submit
open` → the gate-required members → `bb gate --submission <id>`; the deeper
delivery loop is `../bb-dogfood/`.

## Gotchas

- **verify.sh green says nothing about a real run.** Stub harnesses + local substrate; a substrate/harness regression passes the whole gate. Live-run any such change.
- **Sprite exec is WebSocket, never `--http-post`** (cold sprites 502); it resolves org from cwd path history — pin `org/name` host syntax, and confirm org is `misty-step` first.
- **"Re-run it" is not recovery.** Only pre-execute failures retry mechanically; anything at/after execute has side effects and is operator-resolved (`bb runs resolve`, `bb dlq`).
- **Secrets travel on stdin, never argv** — don't add a flag that puts a secret on the command line.

## Report

Return: **verdict** (PASS / FAIL / UNVERIFIED) · exact commands run · surfaces
exercised (machinery vs real remote run) · artifacts inspected (run bundle,
REPORT.json, ledger cost) · what was NOT covered (e.g. "gate only — no live
Sprite run") and the post-ship signal (`bb runs list --json`, `/api/runs`).
