# Bitterblossom Dogfood Notes: Run Telemetry Export

## Context

- Goal: deliver backlog 056, making `bb runs export` a stable telemetry
  contract for Daedalus and OTel-shaped consumers.
- Backlog item: `backlog.d/056-run-telemetry-feedback-loop.md`.
- `bb` binary: `./target/debug/bb`.
- Plane: `plane`.
- Sprite org: `misty-step` after correction.
- Sprite: `lane-1`.
- Commit/submission: pending at note creation.

## Preflight

- `git status`: feature branch `deliver/056-run-telemetry-export`; clean before
  edits, dirty with this delivery during dogfood.
- `flyctl orgs list`: Misty Step available as slug `misty-step`.
- `sprite org list` before: selected org was `adminifi`.
- `sprite use -o misty-step lane-1`: switched successfully.
- `sprite org list` after: selected org is `misty-step`.
- `sprite exec -- whoami`: returned `sprite`.
- `bb check`: loaded 27 tasks and all agents from `plane`.
- `bb status --json` summary: 27 tasks, 0 parked tasks, 5 open DLQ rows,
  cost today `$1.5748238088` against `$25.00`.
- `bb task list --json` summary: 27 tasks, no parked task values.
- `bb dlq list --json` summary: seven historical DLQ rows, five still open.

## Work

- Local changes:
  - Documented `bb.run_telemetry.v1` in `docs/run-telemetry-export-v1.md`.
  - Added fixture and CLI/schema tests in `tests/run_export.rs` and
    `tests/fixtures/run-telemetry-v1.jsonl`.
  - Changed `bb runs export` from raw `{run, attempts}` JSONL to a versioned
    envelope with run, attempts, retry/DLQ, artifact, Daedalus, and OTel fields.
  - Moved the embedded SQLite schema into `src/schema.sql` and bundled it with
    `include_str!` to keep the Rust spine under the LOC cap.
- Local verification:
  - `cargo test --test run_export -- --nocapture`: passed.
  - `cargo test --test ledger -- --nocapture`: passed.
  - `./scripts/verify.sh`: passed; final Rust LOC was 4987.
- Live export dogfood:
  - `./target/debug/bb --config plane runs export | head -n 2`: emitted
    `bb.run_telemetry.v1` rows for `product` and `security`.
  - Parser smoke:
    `bb.run_telemetry.v1 product bitterblossom openrouter` and
    `bb.run_telemetry.v1 security`.
- `bb submit open`: pending.
- `bb run` members: pending.
- `bb gate`: pending.

## UX Notes

### Friction

- Observation: Sprite org state is easy to get wrong; this session started with
  `sprite` selected to `adminifi` while Fly auth exposed `misty-step`.
- Evidence: `sprite org list` showed `Currently selected org: adminifi`;
  `sprite use -o misty-step lane-1` fixed it.
- Mitigate: keep the dogfood preflight requirement; consider making `bb check`
  warn when task workspace hosts imply an org different from the selected
  Sprite org.

- Observation: `bb status --json` is information-rich but too large for quick
  triage in a 27-task plane.
- Evidence: the command returned a huge task array; the useful summary was only
  `tasks=27`, `parked=0`, `open_dlq=5`, `cost_today=$1.5748`.
- Mitigate: add a compact status mode or first-class `bb status summary --json`.

### Bugs

- Observation: no new product bug found in the export dogfood path.
- Evidence: live export parsed cleanly and full verify passed.
- Mitigate: none.

### Delight

- Observation: once the schema existed, `bb runs export` became immediately
  useful against the real historical ledger; no synthetic run was required to
  prove Daedalus/OTel fields.
- Evidence: the first two real rows carried task, cost, token, agent provider,
  Daedalus source, and OTel `gen_ai.*` attributes.
- Lean in: ledger-first observability is a strong product shape; keep exports
  cheap and offline.

## Backlog Emissions

- Added: none yet.
- Updated: backlog 056 will move to `_done/` after `bb` submission/gate.
- Proposed:
  - Warn or fail fast when Sprite selected org differs from the expected plane
    org/host namespace.
  - Add compact status summary command for large planes.

## Closeout

- Final git status: pending.
- Remote sync: pending.
- Remaining parked tasks: 0.
- Remaining DLQ: five open historical rows.
- Next best pickup: pending after submission review.
