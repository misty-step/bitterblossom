# Bitterblossom Dogfood Notes: Run Telemetry Export

## Context

- Goal: deliver backlog 056, making `bb runs export` a stable telemetry
  contract for Daedalus and OTel-shaped consumers.
- Backlog item: `backlog.d/056-run-telemetry-feedback-loop.md`.
- `bb` binary: `./target/debug/bb`.
- Plane: `plane`.
- Sprite org: `misty-step` after correction.
- Sprite: `lane-1`.
- Commit/submission: `ac19f857ce4b50cc7cc36d389b47a64aa6cceb4c`;
  first submission `da50d9baa63c`, replacement submission `9f8333ad1877`.

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
- `bb submit open`:
  - First submission `da50d9baa63c`, change
    `run-telemetry-export-ac19f85`, rev
    `ac19f857ce4b50cc7cc36d389b47a64aa6cceb4c`.
  - Replacement submission `9f8333ad1877` after canonical product member
    `ccc6e9c30c36` failed before emitting a parseable verdict.
- `bb run` members:
  - First submission: `verify` `53798b48e358` pass, `correctness`
    `f4bc4220eec8` pass (`$0.040554035`), `security` `b33cd31c0d96`
    pass (`$0.047172734`), `simplification` `ce44f90c9d0a` pass
    (`$0.0203155689`), `product` `ccc6e9c30c36` failed with
    `pi output: assistant message has no text content`.
  - Replacement submission: `product` `dcc0b206ed31` pass
    (`$0.07269690000000001`), `verify` `415c98665e53` pass,
    `correctness` `81e4cfefb528` pass (`$0.064844986`), `security`
    `43f899dae1f6` pass (`$0.06860631499999999`), `simplification`
    `c716d492b215` advisory (`$0.0128323401`).
- `bb gate`:
  - First gate `da50d9baa63c`: `escalated` because canonical product run
    `ccc6e9c30c36` failed before verdict; all other members passed.
  - Replacement gate `9f8333ad1877`: `clear`; no blocking findings.
    Simplification raised one minor advisory (`b282d4a4571fdbbf`) about
    multiple bounded passes over attempts in `export_run_telemetry`; rejected
    with reason that attempts are bounded by the dispatch retry cap and the
    separate views keep the run/Daedalus/OTel contract explicit.

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

- Observation: the canonical product lane can lose an otherwise-valid verdict
  when the model prints the final JSON through a tool call and then ends with
  no assistant text.
- Evidence: product run `ccc6e9c30c36` reached a tool result containing
  `{"verdict":"pass","findings":[]}`, then failed collection with
  `pi output: assistant message has no text content`.
- Mitigate: clarify verdict-lane prompts to emit the final JSON as assistant
  text, or make the harness parser/reporting surface expose this failure mode
  more directly.

### Delight

- Observation: once the schema existed, `bb runs export` became immediately
  useful against the real historical ledger; no synthetic run was required to
  prove Daedalus/OTel fields.
- Evidence: the first two real rows carried task, cost, token, agent provider,
  Daedalus source, and OTel `gen_ai.*` attributes.
- Lean in: ledger-first observability is a strong product shape; keep exports
  cheap and offline.

## Backlog Emissions

- Added: none in this delivery.
- Updated: backlog 056 moved to `_done/`.
- Proposed:
  - Warn or fail fast when Sprite selected org differs from the expected plane
    org/host namespace.
  - Add compact status summary command for large planes.
  - Harden verdict-lane final-answer behavior so tool-echoed JSON does not
    waste a canonical storm slot without a clearer recovery hint.
  - Consider making `bb gate` safe-next output distinguish infrastructure
    replacement from ordinary blocked-round continuation; after `escalated`,
    `submit open` creates a fresh round-1 replacement for the same change.

## Closeout

- Final git status: pending.
- Remote sync: branch pushed before dogfood storm.
- Remaining parked tasks: 0.
- Remaining DLQ: five open historical rows.
- Next best pickup: compact status / dogfood-friction hardening.
