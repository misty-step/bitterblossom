# Prove event-plane hardening before growing reflex workloads

Priority: P0 | Status: ready | Estimate: XL

## Goal

Make `bb serve` safe enough to be the recurring event plane by removing
credential-leaking auth paths, preventing dispatch starvation, bounding
notification fan-out, and locking the operator contract to live CLI/API
behavior.

## Oracle

- [x] Read API and HTML auth accept bearer headers and reject `?token=`; tests
      and docs cover loopback, public bind, missing token, bad token, and bearer
      success.
- [x] Dispatch in-flight bookkeeping is panic-safe; a regression test proves a
      worker panic cannot strand the task and the next pending run drains.
- [x] Notification execution is bounded or synchronously accounted; a storm
      test proves the process cannot spawn unbounded `curl` waiters.
- [x] CLI/docs/skill parity tests cover `bb run --payload`, no stale `--var`,
      `bb runs export` without `--since`, and selected `--json` examples.
- [x] A minimal generic health/read surface clusters recent runs by task,
      state, reason, cost, duration, parked state, and DLQ status, with safe
      operator actions.
- [ ] Verification includes `./scripts/verify.sh` plus live loopback API/HTML
      QA with and without `BB_API_TOKEN`.

## Children

1. Remove query-token read auth and update docs/skill recipes. (done
   2026-06-13)
2. Add panic-safe in-flight cleanup around run workers. (done 2026-06-14)
3. Bound notification dispatch and synchronously account failed attempts.
   (done 2026-06-14)
4. Add help/doc/skill parity tests for the stale command examples. (done
   2026-06-14)
5. Add the first ledger-native health report needed by operators and agents.
   (done 2026-06-13; see backlog 052)
6. Run a containment/storm drill against the dev plane.

## Notes

Why: product, UX, runtime, architecture, ops, agent-readiness, and premise
challenge lanes all converged here.

Evidence:

- `src/serve.rs:103-140` inserts a task into `in_flight` and removes it only
  after `run_one` returns; worker panic can strand the task until restart.
- `src/serve.rs:209-220` accepted `?token=` and `docs/spine.md:245-247`
  documented it for browsers; fixed 2026-06-13 with bearer-only API/HTML
  auth and live loopback QA.
- `src/notify.rs:16-48` spawns a `curl` process and a detached wait thread per
  notification with no concurrency bound.
- `docs/spine.md:356` still documents `--var`; live `bb run --help` exposes
  `--payload`.
- `docs/spine.md:359` still documents `runs export [--since ...]`; live
  `bb runs export --help` has no `--since`.

Disposition: this epic absorbs the core of 047, 048, and 049 without deleting
those evidence packets.

## Delivery Notes

### 2026-06-13 bearer-only read auth

- Removed query-string read auth from `bb serve`; `/`, `/api/*`, and HTML read
  surfaces now accept only `Authorization: Bearer <BB_API_TOKEN>` when a token
  is configured.
- Added `tests/serve.rs` coverage for missing token, bad bearer, rejected
  `?token=`, successful bearer API, and successful bearer HTML.
- Updated `docs/spine.md` and `skills/bitterblossom/references/operator-recipes.md`
  so consumers no longer learn the unsafe URL-token path.
- Verification: `./scripts/verify.sh` passed with `src LOC: 4991`; live
  loopback QA confirmed missing/bad/query-token requests return `401`, bearer
  `/api/status` returns the status JSON, bearer `/` returns `200`, and no-token
  loopback remains open for dev.

### 2026-06-13 status surface

- `bb status [--json]` and `/api/status` shipped in backlog 052, providing the
  generic health/read surface named by this epic.

### 2026-06-14 panic-safe in-flight cleanup

- Wrapped `bb serve` run workers with panic cleanup: the worker removes its
  task from the in-memory `in_flight` set before resuming an unwind.
- Added `tests/serve.rs` coverage that seeds two pending runs for the same task,
  forces the first worker to panic after dispatch, and proves the second run
  still drains to `success`.
- Verification: `cargo test --test serve
  dispatch_worker_panic_does_not_strand_task_in_flight -- --nocapture` and
  `./scripts/verify.sh` pass.

### 2026-06-14 CLI/docs/skill parity

- Added `tests/cli_contract_docs.rs`, which executes live `bb run --help`,
  `bb runs export --help`, and `bb gate --help`, then checks current
  user-facing docs and skills for the matching examples.
- Updated `skills/bitterblossom/` so consuming agents see `runs export` as the
  no-`--since` telemetry export seam and use the same `<submission>` placeholder
  across submission examples.
- Red proof: `cargo test --test cli_contract_docs -- --nocapture` failed on
  missing `bb --config <plane> runs export` in the portable skill.
- Verification: `cargo test --test cli_contract_docs -- --nocapture` passes.

### 2026-06-14 bounded notification accounting

- Removed the detached waiter thread from `notify::notify`; notifications still
  shell out to `curl -m 10`, but the caller now waits for the child and logs
  non-zero or wait failures before returning.
- Added `tests/budgets.rs` coverage that fires a burst through a slow fake
  notification binary and proves all `curl` children are accounted before
  `notify()` returns.
- Red proof: `cargo test --test budgets notification_storm_is_synchronously_accounted -- --nocapture`
  failed with `left: 0`, `right: 8` because the old implementation returned
  before any waiter finished.
- Focused verification: `cargo test --test budgets -- --nocapture` and
  `cargo test --test submission -- --nocapture` pass.
