use anyhow::{bail, Context, Result};
use serde_json::Value;

use crate::ledger::AttemptStats;
use crate::spec::{AgentSpec, TaskBudget};

pub struct ParsedOutput {
    pub result: String,
    pub stats: AttemptStats,
}

pub const HARNESSES: &[&str] = &["claude", "codex", "pi", "command"];
pub fn build_command(agent: &AgentSpec, budget: &TaskBudget) -> Result<Vec<String>> {
    let bin = agent.bin.clone().unwrap_or_else(|| agent.harness.clone());
    let mut cmd = match agent.harness.as_str() {
        "command" => vec![bin],
        "claude" => {
            let mut c = vec![
                bin,
                "-p".into(),
                "--output-format".into(),
                "json".into(),
                "--model".into(),
                agent.model.clone(),
                "--dangerously-skip-permissions".into(),
            ];
            if let Some(turns) = budget.turn_cap {
                c.push("--max-turns".into());
                c.push(turns.to_string());
            }
            c
        }
        "codex" => vec![
            bin,
            "exec".into(),
            "--json".into(),
            "--dangerously-bypass-approvals-and-sandbox".into(),
            "--model".into(),
            agent.model.clone(),
            "-".into(),
        ],
        "pi" => {
            let mut inner = vec![
                bin,
                "--provider".into(),
                agent.provider().into(),
                "--model".into(),
                agent.model.clone(),
                "--no-session".into(),
                "--mode".into(),
                "json".into(),
                "-p".into(),
            ];
            inner.extend(agent.args.iter().cloned());
            let quoted: Vec<String> = inner
                .iter()
                .map(|a| crate::substrate::local::shell_quote(a))
                .collect();
            return Ok(vec![
                "sh".into(),
                "-c".into(),
                format!(
                    "{{ {pi}; echo $? > .bb-harness-exit; }} \
                     | grep -v -F '\"type\":\"message_update\"'; \
                     exit \"$(cat .bb-harness-exit)\"",
                    pi = quoted.join(" ")
                ),
                "sh".into(),
            ]);
        }
        other => bail!(
            "unknown harness '{other}' (known: {})",
            HARNESSES.join(", ")
        ),
    };
    cmd.extend(agent.args.iter().cloned());
    Ok(cmd)
}
pub fn parse_output(harness: &str, stdout: &str) -> Result<ParsedOutput> {
    match harness {
        "claude" => parse_claude(stdout),
        "codex" => parse_codex(stdout),
        "pi" => parse_pi(stdout),
        "command" => Ok(ParsedOutput {
            result: stdout.trim().to_string(),
            stats: AttemptStats::default(),
        }),
        other => bail!("unknown harness '{other}'"),
    }
}
fn parse_claude(stdout: &str) -> Result<ParsedOutput> {
    let v = last_json_object(stdout).context("claude output: no JSON object found")?;
    if v.get("is_error").and_then(Value::as_bool) == Some(true) {
        bail!("claude reported is_error=true");
    }
    let result = v
        .get("result")
        .and_then(Value::as_str)
        .context("claude output: result object has no 'result' string")?
        .to_string();
    if result.trim().is_empty() {
        bail!("claude output: empty result");
    }
    let usage = v.get("usage").cloned().unwrap_or(Value::Null);
    Ok(ParsedOutput {
        result,
        stats: AttemptStats {
            tokens_in: usage.get("input_tokens").and_then(Value::as_i64),
            tokens_out: usage.get("output_tokens").and_then(Value::as_i64),
            turns: v.get("num_turns").and_then(Value::as_i64),
            cost_usd: v.get("total_cost_usd").and_then(Value::as_f64),
        },
    })
}
fn parse_codex(stdout: &str) -> Result<ParsedOutput> {
    let mut tokens_in: i64 = 0;
    let mut tokens_out: i64 = 0;
    let mut turns: i64 = 0;
    let mut result = String::new();
    let mut saw_event = false;
    for line in stdout.lines() {
        let Ok(v) = serde_json::from_str::<Value>(line) else {
            continue;
        };
        saw_event = true;
        match v.get("type").and_then(Value::as_str) {
            Some("turn.completed") => {
                turns += 1;
                if let Some(u) = v.get("usage") {
                    tokens_in += u.get("input_tokens").and_then(Value::as_i64).unwrap_or(0);
                    tokens_out += u.get("output_tokens").and_then(Value::as_i64).unwrap_or(0);
                }
            }
            Some("item.completed") => {
                if let Some(item) = v.get("item") {
                    if item.get("type").and_then(Value::as_str) == Some("agent_message") {
                        if let Some(text) = item.get("text").and_then(Value::as_str) {
                            result = text.to_string();
                        }
                    }
                }
            }
            _ => {}
        }
    }
    if !saw_event {
        bail!("codex output: no JSONL events found");
    }
    if result.is_empty() {
        bail!("codex output: events present but no agent_message — incomplete run");
    }
    Ok(ParsedOutput {
        result,
        stats: AttemptStats {
            tokens_in: Some(tokens_in),
            tokens_out: Some(tokens_out),
            turns: Some(turns),
            cost_usd: None,
        },
    })
}
fn parse_pi(stdout: &str) -> Result<ParsedOutput> {
    let mut turns: i64 = 0;
    let mut last_message: Option<Value> = None;
    let mut saw_event = false;
    let (mut tokens_in, mut tokens_out, mut cost) = (0i64, 0i64, 0f64);
    let mut saw_usage = false;
    for line in stdout.lines() {
        let Ok(v) = serde_json::from_str::<Value>(line) else {
            continue;
        };
        saw_event = true;
        match v.get("type").and_then(Value::as_str) {
            Some("turn_end") => turns += 1,
            Some("message_end") => {
                if let Some(m) = v.get("message") {
                    if m.get("role").and_then(Value::as_str) == Some("assistant") {
                        if let Some(u) = m.get("usage") {
                            saw_usage = true;
                            tokens_in += u.get("input").and_then(Value::as_i64).unwrap_or(0);
                            tokens_out += u.get("output").and_then(Value::as_i64).unwrap_or(0);
                            cost += u
                                .get("cost")
                                .and_then(|c| c.get("total"))
                                .and_then(Value::as_f64)
                                .unwrap_or(0.0);
                        }
                        last_message = Some(m.clone());
                    }
                }
            }
            _ => {}
        }
    }
    if !saw_event {
        bail!("pi output: no JSONL events found");
    }
    let message = last_message.context("pi output: no assistant message_end — incomplete run")?;
    let result: String = message
        .get("content")
        .and_then(Value::as_array)
        .map(|items| {
            items
                .iter()
                .filter(|c| c.get("type").and_then(Value::as_str) == Some("text"))
                .filter_map(|c| c.get("text").and_then(Value::as_str))
                .collect::<Vec<_>>()
                .join("\n")
        })
        .unwrap_or_default();
    let result = result.trim().to_string();
    if result.is_empty() {
        bail!("pi output: assistant message has no text content");
    }
    Ok(ParsedOutput {
        result,
        stats: AttemptStats {
            tokens_in: saw_usage.then_some(tokens_in),
            tokens_out: saw_usage.then_some(tokens_out),
            turns: Some(turns),
            cost_usd: saw_usage.then_some(cost),
        },
    })
}
fn last_json_object(stdout: &str) -> Option<Value> {
    fn result_from_array(v: Value) -> Option<Value> {
        match v {
            Value::Array(items) => items
                .iter()
                .rev()
                .find(|v| v.get("type").and_then(Value::as_str) == Some("result"))
                .cloned(),
            other => Some(other),
        }
    }
    for line in stdout.lines().rev() {
        let line = line.trim();
        if line.starts_with('{') {
            if let Ok(v) = serde_json::from_str::<Value>(line) {
                return Some(v);
            }
        }
        if line.starts_with('[') {
            if let Ok(v) = serde_json::from_str::<Value>(line) {
                if let Some(result) = result_from_array(v) {
                    return Some(result);
                }
            }
        }
    }
    serde_json::from_str::<Value>(stdout.trim())
        .ok()
        .and_then(result_from_array)
}
