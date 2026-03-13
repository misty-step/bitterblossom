# Walkthrough: Issue 510 Semantic Decision Traces

## Reviewer Evidence

- Artifact: terminal walkthrough in this document
- Protecting check: `python3 -m pytest -q scripts/test_conductor.py -k 'route_issue or route_issues_semantically or invoke_claude_json or semantic_decision'`
- Evidence scope: semantic router invocation metadata, persisted run-level semantic traces, and aggregate metrics visibility

## Walkthrough

### Renderer

Terminal walkthrough

### Core Claim

Bitterblossom now treats semantic routing as a first-class control-plane event: the router uses a named decision-family profile, real runs persist semantic decision traces, `route-issue` exposes preview metadata, and `show-run` / `show-metrics` surface semantic latency and cost separately from builder/reviewer execution telemetry.

### Before

- The semantic router shell-out hard-coded `--model sonnet` inside `invoke_claude_json`.
- No run-scoped ledger recorded which semantic skill/prompt contract ran.
- `show-run` and `show-metrics` could show builder/reviewer telemetry only.

### After

- Semantic routing resolves through a named `issue_routing` decision-family profile.
- Runs persist `semantic_decisions` rows with skill/prompt contract, model/provider, reasoning budget, latency, and cost fields.
- `route-issue` returns `semantic_decision` preview metadata when the router ran.
- `show-run` returns `semantic_decisions` plus semantic summary fields on the run surface.
- `show-metrics` returns semantic summary aggregates across the selected window.

## Evidence

### 1. Focused regression slice

Command:

```bash
python3 -m pytest -q scripts/test_conductor.py -k 'route_issue or route_issues_semantically or invoke_claude_json or semantic_decision'
```

Observed:

```text
......................                                                   [100%]
22 passed, 260 deselected in 0.14s
```

This is the persistent check for the semantic-routing contract.

### 2. `show-run` exposes persisted semantic trace data

Execution: a temp conductor DB was created on this branch, one run was inserted, one semantic decision trace was persisted, and `show-run` was executed against that DB.

Observed excerpt:

```json
{
  "run": {
    "run_id": "run-510-1",
    "semantic_decision_count": 1,
    "semantic_family_usage": [{"family": "issue_routing", "calls": 1}],
    "semantic_average_latency_ms": 118,
    "semantic_estimated_cost_usd": 0.02
  },
  "semantic_decisions": [
    {
      "family": "issue_routing",
      "skill_name": "semantic-router",
      "skill_version": "2026-03-13",
      "prompt_version": "issue-routing-v1",
      "outcome_ref": "issue#510",
      "model": "sonnet",
      "provider": "anthropic",
      "reasoning_budget": "medium",
      "latency_ms": 118,
      "estimated_cost_usd": 0.02
    }
  ]
}
```

### 3. `show-metrics` rolls semantic telemetry up over time

Execution: the same temp DB was queried through `show-metrics --window 7d --limit 5`.

Observed excerpt:

```json
{
  "summary": {
    "semantic_decision_count": 1,
    "semantic_average_latency_ms": 118,
    "semantic_estimated_cost_usd": 0.02,
    "semantic_family_usage": [{"family": "issue_routing", "calls": 1}]
  }
}
```

## Why The New Shape Is Better

- The semantic decision contract is now explicit instead of hidden inside one subprocess helper.
- Routing metadata lives in the run ledger, so operators can inspect it after the fact instead of reconstructing it from logs.
- Runtime telemetry and semantic telemetry are adjacent but separate, which keeps the control plane truthful about what cost came from routing versus execution.

## Residual Gap

- Only the `issue_routing` semantic family is live today. The profile/trace contract is general enough for future semantic phases, but those phases still need to adopt it.
