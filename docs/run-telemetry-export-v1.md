# Run telemetry export v1

`bb runs export` emits newline-delimited JSON. Each line is one ledger run
encoded as `bb.run_telemetry.v1`; consumers should parse each line
independently and skip lines with an unknown `schema`.

The export is the integration seam for Daedalus and future observability
adapters. Bitterblossom does not run an OpenTelemetry or Langfuse sidecar in
the default spine.

## Compatibility

`schema = "bb.run_telemetry.v1"` is the stable contract. Within v1,
Bitterblossom may add nullable fields, object members, and enum values.
It must not rename fields, remove fields, or change numeric units without a
new schema string. Consumers should ignore unknown fields and reject unknown
major schemas.

Timestamps are RFC 3339 UTC strings. Durations are milliseconds. Costs are
USD floats copied from the harness usage report. Token counts are integer
counts reported by the harness/provider and may be `null` when unavailable.

## Envelope

| field | type | meaning |
|---|---|---|
| `schema` | string | Always `bb.run_telemetry.v1`. |
| `schema_version` | integer | Always `1`. |
| `exported_at` | string | Export time, not run time. |
| `run` | object | Normalized run outcome. |
| `attempts` | array | One entry per harness attempt, oldest first. |
| `retry` | object | Mechanical retry summary derived from attempts. |
| `dead_letter` | object | Dead-letter status and replay pointer. |
| `artifacts` | array | Typed artifact pointers available to consumers. |
| `daedalus` | object | Lab handoff shape. |
| `otel` | object | OpenTelemetry GenAI mapping hints. |

## Run

| field | type | meaning |
|---|---|---|
| `id` | string | Bitterblossom run id. |
| `task` | string | Task name from `tasks/<task>/task.toml`. |
| `state` | string | Final or current run state. |
| `state_reason` | string/null | Human-readable state reason. |
| `trigger.kind` | string | Trigger kind, for example `manual`, `webhook`, or `cron`. |
| `trigger.idempotency_key` | string/null | Dedupe key supplied at ingress. |
| `trace_id` | string | Trace id seeded at ingress. |
| `parent_run_id` | string/null | Parent run for replay or child workflows. |
| `agent.name` | string/null | Agent bound at run completion. |
| `agent.version` | integer/null | Agent config version. |
| `config_source.repo` | string/null | Source workload repo when inherited. |
| `config_source.ref` | string/null | Source workload ref when inherited. |
| `cost_usd` | number/null | Total run cost. |
| `tokens.input` | integer/null | Sum of attempt input tokens when any are reported. |
| `tokens.output` | integer/null | Sum of attempt output tokens when any are reported. |
| `duration_ms` | integer/null | Run duration. |
| `created_at` | string | Run creation time. |
| `updated_at` | string | Last state update time. |

## Attempts

Each attempt includes:

| field | type | meaning |
|---|---|---|
| `n` | integer | Attempt number. |
| `phase` | string | Last reached dispatch phase. |
| `outcome` | string/null | Harness outcome. |
| `error` | string/null | Attempt error. |
| `exit_code` | integer/null | Harness process exit code. |
| `agent.name` | string | Agent config name. |
| `agent.version` | integer | Agent config version. |
| `agent.harness` | string | Harness CLI family, for example `pi`, `claude`, `codex`. |
| `agent.model` | string | Model configured for the attempt. |
| `tokens.input` | integer/null | Input tokens. |
| `tokens.output` | integer/null | Output tokens. |
| `turns` | integer/null | Harness turn count. |
| `cost_usd` | number/null | Attempt cost. |
| `artifact_dir` | string/null | Local artifact directory. |
| `started_at` | string | Attempt start time. |
| `ended_at` | string/null | Attempt end time. |

## Retry and DLQ

`retry.attempt_count` is the number of recorded attempts.
`retry.mechanical_retry_count` is `max(attempt_count - 1, 0)`. `retry.final_phase`
is the last attempt phase or `null`.

`dead_letter.status` is `none`, `open`, or `replayed`. Open rows include
`id`, `error`, and `created_at`; replayed rows also include
`replayed_run_id`.

## Daedalus handoff

Daedalus should treat each exported line as an observed control-plane outcome:

```json
{
  "source": "bitterblossom",
  "run_id": "abc123",
  "task_key": "review",
  "trace_id": "trace-abc",
  "agent_configs": [
    {
      "name": "review-coordinator",
      "version": 4,
      "harness": "pi",
      "model": "z-ai/glm-5.2",
      "outcome": "success",
      "cost_usd": 0.03,
      "tokens": { "input": 1200, "output": 450 }
    }
  ],
  "result": {
    "state": "success",
    "state_reason": null,
    "duration_ms": 42000,
    "dead_letter_status": "none"
  }
}
```

Daedalus can join this with launch contracts by `agent_configs[].name`,
`agent_configs[].version`, `harness`, and `model`. Bitterblossom intentionally
does not embed Daedalus scores; score computation belongs in the lab.

## OpenTelemetry GenAI mapping

The export carries an `otel` object so a future adapter can emit spans and
metrics without changing the run contract.

| Bitterblossom field | OTel GenAI target |
|---|---|
| `run.task` | `gen_ai.operation.name` or span name suffix. |
| `attempt.agent.model` | `gen_ai.request.model`, `gen_ai.response.model`. |
| `attempt.agent.name` | `gen_ai.agent.name`. |
| `attempt.agent.version` | `gen_ai.agent.version`. |
| `attempt.tokens.input` | `gen_ai.usage.input_tokens`. |
| `attempt.tokens.output` | `gen_ai.usage.output_tokens`. |
| `attempt.cost_usd` | future cost metric attribute or adapter-local metric. |
| `attempt.started_at`, `attempt.ended_at` | span start/end. |
| `run.duration_ms` | `gen_ai.client.operation.duration` candidate metric. |

Current OTel GenAI conventions live in the OpenTelemetry GenAI semantic
conventions track. The adapter should pin the convention version it emits;
the Bitterblossom v1 export only provides source facts and mapping hints.

## Fixture

`tests/fixtures/run-telemetry-v1.jsonl` is the compatibility fixture. Tests
parse that fixture and live `bb runs export` output to keep docs, schema, and
CLI behavior aligned.
