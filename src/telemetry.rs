use crate::{ledger, spec::Plane};

pub fn export_all(
    plane: &Plane,
    ledger: &ledger::Ledger,
) -> anyhow::Result<Vec<serde_json::Value>> {
    let dead_letters = ledger.list_dead_letters()?;
    let mut out = Vec::new();
    for run in ledger.list_runs(None, None)? {
        let attempts = ledger.attempts(&run.id)?;
        let dlq = dead_letters.iter().find(|d| d.run_id == run.id);
        out.push(export_run(plane, run, attempts, dlq));
    }
    Ok(out)
}

pub fn export_run(
    plane: &Plane,
    r: ledger::RunRow,
    attempts: Vec<ledger::AttemptRow>,
    dlq: Option<&ledger::DeadLetterRow>,
) -> serde_json::Value {
    let dead_status = dlq.map(|d| d.status.as_str()).unwrap_or("none");
    let provider = |name: &str| {
        plane
            .agents
            .get(name)
            .map(|a| a.provider())
            .unwrap_or("unknown")
    };
    let mut attempt_docs = Vec::with_capacity(attempts.len());
    let mut agent_configs = Vec::with_capacity(attempts.len());
    let mut artifacts = Vec::new();
    let mut spans = Vec::with_capacity(attempts.len());
    let (mut input_total, mut output_total) = (0, 0);
    let (mut has_input, mut has_output) = (false, false);
    for a in &attempts {
        let provider = provider(&a.agent_name);
        has_input |= a.tokens_in.is_some();
        has_output |= a.tokens_out.is_some();
        input_total += a.tokens_in.unwrap_or(0);
        output_total += a.tokens_out.unwrap_or(0);
        attempt_docs.push(serde_json::json!({
            "n": a.n, "phase": &a.phase, "outcome": &a.outcome, "error": &a.error,
            "exit_code": a.exit_code,
            "agent": {"name": &a.agent_name, "version": a.agent_version, "harness": &a.harness,
                "provider": provider, "model": &a.model},
            "tokens": {"input": a.tokens_in, "output": a.tokens_out}, "turns": a.turns,
            "cost_usd": a.cost_usd, "artifact_dir": &a.artifact_dir,
            "started_at": &a.started_at, "ended_at": &a.ended_at,
        }));
        agent_configs.push(
            serde_json::json!({"name": &a.agent_name, "version": a.agent_version,
            "harness": &a.harness, "provider": provider, "model": &a.model,
            "outcome": &a.outcome, "cost_usd": a.cost_usd,
            "tokens": {"input": a.tokens_in, "output": a.tokens_out}}),
        );
        if let Some(path) = &a.artifact_dir {
            artifacts.push(
                serde_json::json!({"kind": "attempt_artifact_dir", "attempt": a.n, "path": path}),
            );
        }
        spans.push(serde_json::json!({
            "name": format!("bb.{}.attempt.{}", r.task, a.n), "kind": "internal",
            "start_time": &a.started_at, "end_time": &a.ended_at,
            "attributes": {
                "gen_ai.operation.name": &r.task,
                "gen_ai.provider.name": provider,
                "gen_ai.request.model": &a.model, "gen_ai.response.model": &a.model,
                "gen_ai.agent.name": &a.agent_name,
                "gen_ai.agent.version": a.agent_version.to_string(),
                "gen_ai.usage.input_tokens": a.tokens_in,
                "gen_ai.usage.output_tokens": a.tokens_out,
                "bb.run.id": &r.id, "bb.attempt.n": a.n, "bb.harness": &a.harness
            }
        }));
    }
    let input_tokens = has_input.then_some(input_total);
    let output_tokens = has_output.then_some(output_total);
    let dead_letter = dlq.map_or_else(
        || serde_json::json!({"status": "none"}),
        |d| {
            serde_json::json!({"status": dead_status, "id": d.id, "error": &d.error,
            "created_at": &d.created_at, "replayed_run_id": &d.replayed_run_id,
            "acknowledged_reason": &d.acknowledged_reason, "acknowledged_at": &d.acknowledged_at})
        },
    );
    serde_json::json!({
        "schema": "bb.run_telemetry.v1", "schema_version": 1, "exported_at": ledger::now(),
        "run": {"id": &r.id, "task": &r.task, "state": &r.state, "state_reason": &r.state_reason,
            "trigger": {"kind": &r.trigger_kind, "idempotency_key": &r.idempotency_key},
            "trace_id": &r.trace_id, "parent_run_id": &r.parent_run_id,
            "agent": {"name": &r.agent_name, "version": r.agent_version},
            "config_source": {"repo": &r.config_source_repo, "ref": &r.config_source_ref},
            "cost_usd": r.cost_usd, "tokens": {"input": input_tokens, "output": output_tokens},
            "duration_ms": r.duration_ms, "created_at": &r.created_at, "updated_at": &r.updated_at},
        "attempts": attempt_docs,
        "retry": {"attempt_count": attempts.len(), "mechanical_retry_count": attempts.len().saturating_sub(1),
            "final_phase": attempts.last().map(|a| a.phase.as_str())},
        "dead_letter": dead_letter, "artifacts": artifacts,
        "daedalus": {"source": "bitterblossom", "run_id": &r.id, "task_key": &r.task,
            "trace_id": &r.trace_id, "agent_configs": agent_configs,
            "result": {"state": &r.state, "state_reason": &r.state_reason,
                "duration_ms": r.duration_ms, "dead_letter_status": dead_status}},
        "otel": {"trace_id": &r.trace_id, "spans": spans,
            "metrics": [{"name": "gen_ai.client.operation.duration", "unit": "ms",
                "value": r.duration_ms, "attributes": {"gen_ai.operation.name": &r.task, "bb.run.id": &r.id}}]},
    })
}
