# 033 Raindrop Trial Plan

Backlog item:
[033-raindrop-production-error-ingress](/backlog.d/033-raindrop-production-error-ingress.md)

Date: 2026-06-13

## Decision So Far

Do not archive backlog 033 yet. The live trial has not happened, and the
ticket oracle requires one instrumented AI-feature product plus a week of
signal before the keep/drop verdict is honest.

## Current Raindrop Shape

Sources refreshed on 2026-06-13:

- Raindrop introduction: <https://raindrop.ai/docs/introduction>
- Workshop docs: <https://www.raindrop.ai/docs/workshop/overview/>
- Workshop repository: <https://github.com/raindrop-ai/workshop>
- Python SDK: <https://www.raindrop.ai/docs/sdk/python/>
- TypeScript SDK: <https://www.raindrop.ai/docs/sdk/typescript/>
- HTTP API: <https://www.raindrop.ai/docs/sdk/http-api/>
- MCP overview: <https://www.raindrop.ai/docs/mcp/overview/>
- PII redaction: <https://www.raindrop.ai/docs/security/pii-redaction/>

Relevant facts:

- Hosted ingestion needs a Raindrop write key. The HTTP API uses
  `Authorization: Bearer YOUR_WRITE_KEY` against
  `https://api.raindrop.ai/v1`.
- Python and TypeScript SDKs track user events and AI interactions; Python
  can auto-instrument LLM client libraries when tracing is enabled.
- Workshop is the local debugger path. The documented install is
  `curl -fsSL https://raindrop.sh/install | bash`, and the `raindrop`
  binary can run `raindrop workshop`, `raindrop workshop setup`, and
  `raindrop cloud setup`.
- The MCP server is hosted at `https://mcp.raindrop.ai/mcp` and lets a
  coding assistant ask Raindrop Triage for issues and traces.
- PII Guard is ingestion-side redaction, with a paid base tier and event
  usage tiers. If cloud is used, the trial must explicitly avoid sending
  secrets or sensitive user content.

## Local Prerequisite Check

Commands run:

```bash
command -v raindrop || true
command -v workshop || true
command -v raindrop-workshop || true
env | rg 'RAINDROP|OPENROUTER|GH_TOKEN|SENTRY|LANGFUSE|WORKSHOP'
rg -n "Raindrop|raindrop|RAINDROP|Workshop|workshop" -S . ../harness-kit ../linejam ../canary ../curb ../sploot ../conviction
```

Result:

- No Raindrop/Workshop CLI is installed.
- No `RAINDROP_WRITE_KEY`, `RAINDROP_API_KEY`, or other Raindrop credential
  is present in this shell.
- No existing Raindrop instrumentation was found in this repo or the sampled
  nearby product repos.

## Trial Target

Use Bitterblossom's review factory as the first product.

Why this target:

- It is an actual AI-feature workload, not a synthetic demo.
- It already produces real ledger rows, cost data, PR comments, and run
  artifacts.
- Backlog 034 exposed behavior-level failures that exception logging alone
  would not classify well: clone/fetch timeout, hidden final JSON, and shell
  interpolation damaging review markdown.
- The source of truth is local and operator-owned, so the trial can preserve
  the event-plane principle that workload judgment stays out of the Rust
  spine.

## Signals

Start with three behavior-level signals:

- `review_output_unparseable`: final harness output cannot be parsed into the
  visible JSON contract.
- `review_comment_contract_breach`: measurement run posts a comment, normal
  run fails to post one comment, or the posted body differs from `REVIEW.md`.
- `review_unbounded_source_access`: reviewer attempts clone/fetch/checkout or
  times out while expanding source scope.

Each signal should carry: run id, PR, agent name/version, model, cost,
duration, failure phase, artifact path or trace URL, and a sanitized excerpt.

## Instrumentation Sketch

Preferred first slice after credentials exist:

1. Add an env-gated `bb runs export-raindrop` or post-attempt exporter that
   emits a small event per review attempt through Raindrop's HTTP API.
2. Keep the Rust spine generic: event export consumes ledger rows and attempt
   artifacts; no review-specific branch in dispatch, substrate, or harness.
3. For local-only Workshop, run `raindrop workshop setup` in a product repo
   and prove that one review-factory attempt trace is inspectable locally.
4. For cloud trial, set `RAINDROP_WRITE_KEY` only in the operator
   environment or Fly secrets; never commit it.

## Week-Long Trial Gate

Run for seven calendar days after instrumentation starts.

Evidence to collect:

- At least one Raindrop issue or explicit "no issue surfaced" finding.
- Comparison against the degenerate alternative: ledger JSON plus scheduled
  triage over `bb runs list --task review --json`.
- Setup and operating cost: subscription tier, PII Guard choice, event volume,
  and operator time.
- Keep/drop verdict:
  - Keep only if Raindrop surfaces a useful behavior-level issue or materially
    improves trace-to-fix speed over the ledger triage loop.
  - Drop if the local ledger plus scheduled triage gives comparable signal
    with lower cost and fewer vendor/PII concerns.

## Blockers

- Need a product owner decision to use the review factory as the trial target
  or name a different AI-feature product.
- Need Raindrop access: either cloud credentials (`RAINDROP_WRITE_KEY`) or an
  explicit decision to install Workshop locally first.
- Need elapsed trial time. The backlog oracle requires a week of signal; a
  same-day docs review cannot close it.
