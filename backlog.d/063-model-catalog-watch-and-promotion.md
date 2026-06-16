# Watch model catalogs and promote agent configs safely

Priority: P1 | Status: ready | Estimate: M | Shaped: yes

## PRD Summary

- User: the Bitterblossom operator and the ad-hoc agents changing plane agent
  configs.
- Problem: provider model catalogs change faster than repo memory; a launch can
  be page-visible before it is API-runnable, or API-runnable before our checked
  configs and model-eval docs notice.
- Goal: make model freshness a repeatable, reviewable workflow that detects
  catalog/config drift and recommends promotions without silently mutating
  production reflex defaults.
- Why now: GLM 5.2 landed on OpenRouter on June 16, 2026, and the repo needed
  a manual correction from GLM 5.1 to GLM 5.2.
- UX enabled: an operator can ask "are our configured OpenRouter models current
  and valid?" and get a deterministic answer plus a recommendation artifact for
  new candidates.
- Deliverable type: working code, task config, docs, and a report-producing
  reflex workload.
- Success signal: `./scripts/verify.sh` includes an offline catalog fixture
  guard, while `bb --config plane run model-catalog-watch --payload
  '{"dry_run":true}' --json` produces a `REPORT.json` recommendation instead of
  changing configs.

## Product Requirements

- P0: Validate every configured `pi` + OpenRouter model id in `plane/agents/*.toml`
  against a catalog document that has model id, name, context length, pricing,
  modalities, supported parameters when available, and provider metadata.
- P0: Do not put live network access into the default local gate. The CI/local
  guard must run against a checked-in fixture so developers are not blocked by
  provider/API outages.
- P0: Add a manual and scheduled `model-catalog-watch` task that fetches the
  live OpenRouter catalog, compares it with the checked-in fixture/config/docs,
  and writes a recommendation report. It may file a backlog PR only when not in
  `dry_run`.
- P0: Promotion remains explicit: no task, script, or check edits
  `plane/agents/*.toml` automatically. A promotion needs a `bb` smoke run for
  the affected flow and a model-eval reference record before changing defaults.
- P1: The watcher should flag likely successor candidates by family, not only
  missing ids. Initial families are the families already represented in
  `plane/agents`: DeepSeek, Kimi/Moonshot, GLM/Z.ai, Grok/xAI, OpenAI.
- P1: The report should include copyable next commands for the relevant
  first-class model-eval cohort when a candidate looks runnable.
- Non-goals: broad benchmark ranking, automatic default promotion, broad web
  scraping of provider launch posts, OpenRouter spend optimization across all
  public models, a Rust `bb model-catalog` subcommand in this slice, or provider
  support beyond OpenRouter unless it fits the same fixture shape.

## Technical Design

- Chosen architecture: a hybrid guard plus reflex.
  - Guard: add `scripts/check-model-catalog.sh` with fixture and live modes. The
    default verification path uses a checked-in OpenRouter fixture under
    `tests/fixtures/`.
  - Reflex: add `plane/tasks/model-catalog-watch/` and
    `plane/agents/model-catalog-watcher.toml`. The card fetches the live
    OpenRouter catalog, compares it to config/docs/fixture, and writes
    `REPORT.json` with discoveries and promotion recommendations.
  - Docs: update `docs/model-evals/README.md` with the promotion policy and
    the catalog-fixture boundary.
- Files/systems touched:
  - `scripts/check-model-catalog.sh`
  - `tests/model_catalog.rs`
  - `tests/fixtures/openrouter-models-current.json`
  - `plane/agents/model-catalog-watcher.toml`
  - `plane/tasks/model-catalog-watch/task.toml`
  - `plane/tasks/model-catalog-watch/card.md`
  - `docs/model-evals/README.md`
  - `docs/spine.md`
  - `scripts/verify.sh`
- Data/control flow:
  1. The script reads OpenRouter catalog JSON from a fixture path or from
     `https://openrouter.ai/api/v1/models` in live mode.
  2. It extracts configured OpenRouter model ids from `plane/agents/*.toml`.
  3. It fails if any configured model id is absent from the catalog or lacks
     required metadata.
  4. It emits human text by default and stable JSON under `--json`.
  5. The watcher task runs the same check against live data, then adds release
     and candidate-comparison judgement in `REPORT.json`.
  6. The operator or a later builder lane performs the actual promotion through
     the existing model-eval loop.
- Build/check boundary:
  - Build/local verification catches missing configured ids using the fixture.
  - Live scheduled/manual verification catches new provider releases, price or
    context changes, and fixture drift.
- ADR decision: not required. This stays inside ADR-005's accepted event-plane
  boundary: workloads are files, the spine holds no model-selection judgement.
  Escalate to an ADR only if the implementation wants provider polling or model
  promotion logic inside `src/`.
- ADR-style invariants:
  - No workload-specific branch in dispatch, substrate, or queue. Violation
    recreates the bloat ADR-005 rejected.
  - `./scripts/verify.sh` must stay deterministic without live network.
    Violation makes local completion depend on provider uptime.
  - Promotions require smoke plus model-eval evidence. Violation turns catalog
    launch hype into production defaults.
  - Rust spine LOC budget remains <= 5000. Current `src` LOC was 4981 when
    shaped, so new Rust surface must delete or consolidate first.

## CLI Surface

- Command tree: repo script, not a `bb` subcommand in this slice.
- Usage:
  - `scripts/check-model-catalog.sh --catalog tests/fixtures/openrouter-models-current.json`
  - `scripts/check-model-catalog.sh --live --json`
- Args/flags:
  - `--catalog <path>`: read catalog JSON from a file.
  - `--live`: fetch `https://openrouter.ai/api/v1/models`.
  - `--agents <dir>`: default `plane/agents`.
  - `--docs <path>`: default `docs/model-evals/README.md`.
  - `--json`: machine-readable report.
- Output contract:
  - Human mode prints configured model count, missing ids, metadata gaps, and
    candidate discoveries.
  - JSON mode includes `status`, `provider`, `catalog_source`, `configured`,
    `missing`, `metadata_gaps`, `new_family_candidates`, and
    `promotion_required_evidence`.
- Error/exit code map:
  - `0`: configured models exist and required metadata is present.
  - `1`: configured model missing, malformed catalog, or required metadata gap.
  - `2`: usage error or live fetch failure.
- Config/env precedence: flags > defaults. `OPENROUTER_API_KEY` is optional for
  catalog fetch and must not be required for the offline fixture path.
- Safety controls: no write mode in the check script. The watcher defaults to
  `dry_run = true` for model-eval variants and only files backlog PRs when an
  explicit non-dry-run payload is supplied.
- Examples:
  - Happy path: `scripts/check-model-catalog.sh --catalog
    tests/fixtures/openrouter-models-current.json --json | jq -e
    '.status == "pass"'`
  - Live probe: `scripts/check-model-catalog.sh --live --json | jq -e
    '.configured[] | select(.id == "z-ai/glm-5.2")'`

## Lead Repo Read

- `backlog.d/063-model-catalog-watch-and-promotion.md`: original premise and
  candidate approaches.
- `project.md`: product boundary, target user, event-plane primitives.
- `docs/spine.md`: model/auth policy, model-eval loop, CLI contract, submission
  loop, and current GLM 5.2 record.
- `docs/adr/005-rust-event-plane.md`: Rust spine must stay small; workloads are
  files, not runtime code.
- `src/spec.rs`: plane loads `agents/*.toml` and task configs; provider defaults
  to OpenRouter for `pi`.
- `src/main.rs`: current CLI has no model-catalog command, and `bb check` only
  loads config.
- `plane/tasks/gardener/` and `plane/tasks/model-eval/`: report-writing task
  and candidate-comparison conventions to follow.
- `tests/model_eval.rs` and `docs/model-evals/README.md`: current checked model
  cohort surface and promotion doctrine.
- `scripts/verify.sh`: the one repo gate, including the <= 5000 `src` LOC
  budget.
- External source: OpenRouter OpenAPI `/models` operation and live
  `https://openrouter.ai/api/v1/models` response.

## Alternatives

| Option | Why it helps | Failure mode | Verdict |
|---|---|---|---|
| Manual daily/weekly catalog sweep | Cheapest and flexible. | Depends on operator memory; repeats the GLM 5.2 miss. | Reject as the main path; keep as emergency fallback. |
| Pure scheduled LLM watcher | Fits the event plane and can produce readable recommendations. | Too soft for configured-id correctness; a malformed report could look useful. | Reject alone; keep as the recommendation layer. |
| Live network check in `./scripts/verify.sh` | Catches stale ids against reality on every CI run. | Provider outage or rate limit blocks unrelated local work; bad developer UX. | Reject for default gate. |
| Rust `bb model-catalog check` | Most first-class operator surface. | `src` has only 19 LOC of budget headroom; provider polling in the spine risks ADR-005 drift. | Defer until a refactor earns it. |
| Hybrid offline guard plus live reflex | Deterministic local safety plus durable live discovery. | Requires fixture refresh discipline and one new task/card. | Choose. |

## Alignment Questions

- Should the first provider be OpenRouter only? Recommended answer: yes.
  Evidence: all current `pi` agents default to OpenRouter, and the GLM miss was
  an OpenRouter catalog issue. Risk if wrong: provider-general abstractions
  slow the first fix.
- Should the script auto-update the fixture? Recommended answer: no for this
  slice. It may print a refresh command, but writes should be explicit. Risk if
  wrong: silent fixture churn hides real catalog changes.
- Should the watcher file PRs automatically? Recommended answer: only when
  `dry_run` is explicitly false and it has a concrete candidate or drift
  finding. Risk if wrong: daily noise PRs.
- Should release pages or RSS outrank the API catalog? Recommended answer: no.
  They are discovery signals. Runnable status comes from the API catalog and a
  `bb` smoke. Risk if wrong: page-visible/API-pending models become false
  positives again.

## Oracle

Commands that must all exit 0 for delivery:

- `cargo test --test model_catalog -- --nocapture`
- `scripts/check-model-catalog.sh --catalog tests/fixtures/openrouter-models-current.json --json | jq -e '.status == "pass"'`
- `scripts/check-model-catalog.sh --live --json | jq -e '.configured[] | select(.id == "z-ai/glm-5.2")'`
- `cargo test --test model_eval -- --nocapture`
- `./scripts/verify.sh`
- `bb --config plane run model-catalog-watch --payload '{"dry_run":true}' --json`

Observable outcomes:

- The dry-run watcher `REPORT.json` names configured models, any live catalog
  deltas, recommended follow-up, and the promotion evidence required before
  changing defaults.
- `bb --config plane check` lists the new `model-catalog-watch` task and keeps
  all existing model-eval cohorts valid.
- If live OpenRouter is unavailable during delivery, the builder must stop or
  record an explicit live-fetch blocker; do not substitute stale memory for the
  live probe.

## Premise Source

Premise Source: sha256:6fd658002c6593cf1b9a2670bf5b691b1fa7e7ab49618f0da2463c4cb5ec8b33 git:3bc4b79d2380026d78d226d5f6dd231c72d10596:backlog.d/063-model-catalog-watch-and-promotion.md

## HTML Plan

`docs/plans/2026-06-16-063-model-catalog-watch.html`

## Risks + Rollout

- Risk: the script becomes an ad hoc parser for TOML and drifts. Mitigation:
  test against representative agent fixtures and keep parsing narrow: extract
  `model`, `harness`, and `provider` from current simple agent files only.
- Risk: live catalog fields change. Mitigation: fail closed with a clear
  malformed-catalog error and preserve the raw field path in JSON output.
- Risk: the watcher recommends a model by novelty. Mitigation: report
  recommendations are candidates only; promotion still requires flow-specific
  smoke and model-eval evidence.
- Risk: notification noise. Mitigation: cron task reports one artifact and one
  optional backlog PR, deduped by provider/catalog snapshot hash.
- Rollback: remove the watcher task/agent and script hook from
  `scripts/verify.sh`; existing agent configs and model-eval docs remain
  unchanged.

## Stop Conditions

- Implementation needs more than a small script/test/task/doc slice or wants
  provider polling in Rust without deleting enough spine LOC.
- OpenRouter live catalog is unavailable and no live evidence can be captured.
- The watcher cannot produce deterministic JSON evidence or starts editing
  `plane/agents` directly.
- A candidate promotion lacks a successful `bb` smoke run and model-eval
  record.
