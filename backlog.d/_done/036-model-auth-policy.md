# Enforce the model & auth policy in the plane, not in prose

Priority: P0 · Status: done · Estimate: L

## Goal
The plane mechanically enforces who may spend what: event-triggered
workloads run cheap open-weight models via OpenRouter on open harnesses;
claude/codex run only on subscription auth, never API keys — a
misconfigured task fails `bb check`, it does not quietly burn money.

## Oracle
- [x] `agents/<name>.toml` carries the binding explicitly (provider /
      auth fields), and `bb check` fails a plane where a task with a
      webhook or cron trigger binds an agent whose model is an
      Anthropic/OpenAI API model
- [x] Dispatch refuses to start a claude/codex harness run when
      `ANTHROPIC_API_KEY` / `OPENAI_API_KEY` would reach the workload
      env; subscription (OAuth) auth is the only path for those
      harnesses — test proves the key never crosses the exec boundary
- [x] The pi adapter passes the agent's model through (today
      `harness.rs` builds pi's argv with no `--model` — the binding is
      silently ignored) and an OpenRouter-bound agent
      (e.g. `moonshotai/kimi-k2.6` or `deepseek/v4-flash`) completes a
      real run with cost/tokens in the ledger
- [x] The review agent is rebound to a cheap model and a seeded-flaw PR
      still gets all plantings flagged (the 034 quality gate), with
      per-run cost in the ledger at a fraction of the $2.46–3.09 claude
      baseline

## Children
1. Spec: `provider` + `auth = "subscription" | "api"` on AgentSpec;
   trigger-class policy in plane.toml (`[policy]` — allowed
   harness/auth per trigger kind); `bb check` enforcement.
2. Dispatch env hygiene: explicit allowlist of env vars that cross into
   the workload exec; anthropic/openai API keys never on it.
3. pi adapter: model passthrough + OpenRouter provider env
   (`OPENROUTER_API_KEY` as an agent secret); verify pi headless JSON
   output parses (we have the parser, never ran it live). Evaluate
   goose/opencode only if pi falls short — one open harness is enough.
4. Rebind `plane/agents/review-coordinator.toml` to the cheap stack;
   re-run the seeded-flaw PR; record cost. (Feeds 034's median oracle.)

## Notes
Operator direction 2026-06-10. Verified market facts (2026-06,
parallel-search): DeepSeek V4 Flash $0.14/$0.28 per 1M via OpenRouter;
Kimi K2.6 ~$0.95/$4.00 (262k ctx, tool calling, agentic, SWE-Bench Pro
~58.6%); GLM-5.1 ~$1.10 out; MiniMax M2.7 $1.20 out — vs Opus 4.7 at
$75/1M out. Model facts rot; re-verify at delivery. Note the "Anthropic
tax": Anthropic discourages subscription auth from third-party
harnesses, which is exactly why claude-on-subscription stays an ad-hoc
(operator-initiated) privilege and event workloads go OpenRouter.

## Evidence (2026-06-11)
- Policy at load: tests/policy.rs (claude+api rejected, forbidden
  secrets, reflex-requires-api, pi defaults). Hermetic exec proven in
  tests/e2e_local.rs + tests/e2e_sprites.rs (env scrubbed both
  substrates, HOME relocated, declared secrets only).
- pi adapter: provider/model passthrough, JSONL parse, usage summed
  per-call, message_update flood filtered (718 MB observed pre-filter),
  exit code preserved. Live: review-coordinator@v2 on Kimi K2.6 —
  $0.0034-0.03/review vs $2.46-3.09 claude baseline; seeded-flaw PR #843
  still fully flagged (SQL injection + unquoted rm -rf caught).
- Codex adversarial review round (receipt
  20260611T030135-codex-35cecb5e): 2 blocking + 2 serious, all fixed
  with regression tests.
