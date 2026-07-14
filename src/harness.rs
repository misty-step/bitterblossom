use anyhow::{bail, Context, Result};
use serde_json::Value;

use crate::ledger::AttemptStats;
use crate::spec::{AgentSpec, TaskBudget};
use crate::substrate::CARD_FILENAME;

pub struct ParsedOutput {
    pub result: String,
    pub stats: AttemptStats,
}

#[derive(Clone, Default)]
pub struct PartialProgress {
    pub stats: AttemptStats,
    pub tool_actions: Option<i64>,
}

pub const HARNESSES: &[&str] = &["claude", "codex", "pi", "omp", "command", "opencode"];

/// Whether a successful run on this harness reports dollar cost into
/// `cost_usd` (bitterblossom-969). Empirical, from the parser contracts in
/// this file plus live ledger receipts: `claude` reports `total_cost_usd` on
/// the final result JSON; `pi`/`omp` report `usage.cost.total` on assistant
/// `message_end` events (OpenRouter per-response usage accounting);
/// `opencode` reports `cost` on `step-finish`. `codex` JSONL carries token
/// usage only — no dollar figure exists on its subscription surface, so
/// `parse_codex` records `cost_usd = None` by construction. `command`
/// wrappers MAY self-report via `bb.command_result.v1`, but nothing
/// guarantees it, so the plane must treat the harness as cost-blind.
pub fn reports_cost(harness: &str) -> bool {
    matches!(harness, "claude" | "pi" | "omp" | "opencode")
}

/// The uniform commission preamble every dispatched lane receives, on every
/// harness and substrate. bitterblossom-971: the refused-credential rule is
/// plane security policy (like env scrubbing and `unset GH_TOKEN`), not
/// workload judgment — a 401/403 on a declared credential bounds the lane's
/// blast radius by design, and the 2026-07-09 incident showed lanes will
/// otherwise treat it as a puzzle and locate operator admin credentials.
/// Doctrine: docs/credential-refusal-doctrine.md.
pub fn commission_prompt() -> String {
    format!(
        "Read {CARD_FILENAME} in this directory — it is your entire commission. Execute it. \
         If a credential this run declares is refused (HTTP 401/403 or equivalent), that is a \
         STOP-and-report boundary: write REPORT.json naming the refused operation and stop; \
         never locate or use a stronger credential."
    )
}
pub fn build_command(agent: &AgentSpec, budget: &TaskBudget) -> Result<Vec<String>> {
    if effective_tool_action_cap(agent, budget).is_some()
        && !supports_tool_action_cap(&agent.harness)
    {
        bail!(
            "tool_action_cap is not enforceable for harness '{}' before execution",
            agent.harness
        );
    }
    let turn_cap = effective_turn_cap(agent, budget);
    if turn_cap.is_some() && !supports_turn_cap(&agent.harness) {
        bail!(
            "turn_cap/iteration_cap is not enforceable for harness '{}' before execution",
            agent.harness
        );
    }
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
            if let Some(turns) = turn_cap {
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
                // Bitterblossom-918: a global pi extension (observed:
                // ops-watchdog) can register a recurring sampler that
                // outlives `--no-session` teardown, throwing the SDK's
                // stale-context guard after the model response already
                // succeeded. Every bounded bb dispatch is a one-shot,
                // non-interactive commission that gains nothing from a
                // personal pi extension, so extension discovery is off by
                // default here rather than per-agent-config opt-out. This is
                // the operator's own recorded workaround, pending the
                // upstream fix (pi-agent-config#23, deliberately unmerged —
                // pi-agent-config is being retired in favor of Roster).
                // Remove once bb no longer dispatches through `pi`.
                "--no-extensions".into(),
                "--mode".into(),
                "json".into(),
                "-p".into(),
            ];
            inner.extend(agent.args.iter().cloned());
            return Ok(filtered_jsonl_command(inner));
        }
        "omp" => {
            let mut inner = vec![
                bin,
                "--provider".into(),
                agent.provider().into(),
                "--model".into(),
                agent.model.clone(),
                "--no-session".into(),
                "--mode".into(),
                "json".into(),
                "--auto-approve".into(),
            ];
            inner.extend(agent.args.iter().cloned());
            inner.push("-p".into());
            inner.push(commission_prompt());
            return Ok(filtered_jsonl_command(inner));
        }
        "opencode" => {
            let mut inner = vec![
                bin,
                "run".into(),
                commission_prompt(),
                "--format".into(),
                "json".into(),
                "--model".into(),
                format!("{}/{}", agent.provider(), agent.model),
            ];
            inner.extend(agent.args.iter().cloned());
            return Ok(inner);
        }
        other => bail!(
            "unknown harness '{other}' (known: {})",
            HARNESSES.join(", ")
        ),
    };
    cmd.extend(agent.args.iter().cloned());
    Ok(cmd)
}

fn effective_turn_cap(agent: &AgentSpec, budget: &TaskBudget) -> Option<u32> {
    [
        budget.turn_cap,
        agent.policy.turn_cap,
        agent.policy.iteration_cap,
    ]
    .into_iter()
    .flatten()
    .min()
}

fn effective_tool_action_cap(agent: &AgentSpec, budget: &TaskBudget) -> Option<u32> {
    [budget.tool_action_cap, agent.policy.tool_action_cap]
        .into_iter()
        .flatten()
        .min()
}

fn supports_tool_action_cap(harness: &str) -> bool {
    matches!(harness, "codex" | "pi" | "omp")
}

fn supports_turn_cap(harness: &str) -> bool {
    matches!(harness, "claude" | "codex" | "pi" | "omp")
}

pub fn parse_output(harness: &str, stdout: &str) -> Result<ParsedOutput> {
    match harness {
        "claude" => parse_claude(stdout),
        "codex" => parse_codex(stdout),
        "pi" | "omp" => parse_agent_jsonl(harness, stdout),
        "command" => parse_command(stdout),
        "opencode" => parse_opencode(stdout),
        other => bail!("unknown harness '{other}'"),
    }
}

pub fn parse_partial_stats(harness: &str, stdout: &str) -> AttemptStats {
    match harness {
        "pi" | "omp" => partial_agent_jsonl_stats(stdout),
        "claude" => partial_claude_stats(stdout),
        "codex" => partial_codex_stats(stdout),
        "command" => partial_command_stats(stdout),
        "opencode" => partial_opencode_stats(stdout),
        _ => AttemptStats::default(),
    }
}

pub fn parse_partial_progress(harness: &str, stdout: &str) -> PartialProgress {
    PartialProgress {
        stats: parse_partial_stats(harness, stdout),
        tool_actions: match harness {
            "codex" | "pi" | "omp" | "opencode" => partial_tool_actions_jsonl(stdout),
            "claude" | "command" => partial_tool_actions_last_json(stdout),
            _ => None,
        },
    }
}

fn partial_tool_actions_jsonl(stdout: &str) -> Option<i64> {
    let mut count = 0;
    for line in stdout.lines() {
        let Ok(v) = serde_json::from_str::<Value>(line) else {
            continue;
        };
        count += count_tool_actions(&v);
    }
    (count > 0).then_some(count)
}

fn partial_tool_actions_last_json(stdout: &str) -> Option<i64> {
    let count = last_json_object(stdout)
        .as_ref()
        .map(count_tool_actions)
        .unwrap_or(0);
    (count > 0).then_some(count)
}

fn count_tool_actions(value: &Value) -> i64 {
    match value {
        Value::Object(map) => {
            let here = map
                .get("type")
                .and_then(Value::as_str)
                .is_some_and(is_tool_action_type) as i64;
            here + map.values().map(count_tool_actions).sum::<i64>()
        }
        Value::Array(items) => items.iter().map(count_tool_actions).sum(),
        _ => 0,
    }
}

fn is_tool_action_type(kind: &str) -> bool {
    let kind = kind.to_ascii_lowercase();
    kind.contains("tool") && (kind.contains("call") || kind.contains("use"))
}

fn partial_agent_jsonl_stats(stdout: &str) -> AttemptStats {
    let mut turns: i64 = 0;
    let (mut tokens_in, mut tokens_out, mut cost) = (0i64, 0i64, 0f64);
    let mut saw_usage = false;
    for line in stdout.lines() {
        let Ok(v) = serde_json::from_str::<Value>(line) else {
            continue;
        };
        if v.get("type").and_then(Value::as_str) == Some("turn_end") {
            turns += 1;
        }
        if v.get("type").and_then(Value::as_str) != Some("message_end") {
            continue;
        }
        let Some(message) = v.get("message") else {
            continue;
        };
        if message.get("role").and_then(Value::as_str) != Some("assistant") {
            continue;
        }
        let Some(usage) = message.get("usage") else {
            continue;
        };
        saw_usage = true;
        tokens_in += usage.get("input").and_then(Value::as_i64).unwrap_or(0);
        tokens_out += usage.get("output").and_then(Value::as_i64).unwrap_or(0);
        cost += usage
            .get("cost")
            .and_then(|c| c.get("total"))
            .and_then(Value::as_f64)
            .unwrap_or(0.0);
    }
    AttemptStats {
        tokens_in: saw_usage.then_some(tokens_in),
        tokens_out: saw_usage.then_some(tokens_out),
        turns: (turns > 0).then_some(turns),
        cost_usd: saw_usage.then_some(cost),
    }
}

fn partial_claude_stats(stdout: &str) -> AttemptStats {
    let Some(v) = last_json_object(stdout) else {
        return AttemptStats::default();
    };
    let usage = v.get("usage").cloned().unwrap_or(Value::Null);
    AttemptStats {
        tokens_in: usage.get("input_tokens").and_then(Value::as_i64),
        tokens_out: usage.get("output_tokens").and_then(Value::as_i64),
        turns: v.get("num_turns").and_then(Value::as_i64),
        cost_usd: v.get("total_cost_usd").and_then(Value::as_f64),
    }
}

fn partial_codex_stats(stdout: &str) -> AttemptStats {
    let mut tokens_in: i64 = 0;
    let mut tokens_out: i64 = 0;
    let mut turns: i64 = 0;
    let mut saw_usage = false;
    for line in stdout.lines() {
        let Ok(v) = serde_json::from_str::<Value>(line) else {
            continue;
        };
        if v.get("type").and_then(Value::as_str) != Some("turn.completed") {
            continue;
        }
        turns += 1;
        if let Some(usage) = v.get("usage") {
            saw_usage = true;
            tokens_in += usage
                .get("input_tokens")
                .and_then(Value::as_i64)
                .unwrap_or(0);
            tokens_out += usage
                .get("output_tokens")
                .and_then(Value::as_i64)
                .unwrap_or(0);
        }
    }
    AttemptStats {
        tokens_in: saw_usage.then_some(tokens_in),
        tokens_out: saw_usage.then_some(tokens_out),
        turns: (turns > 0).then_some(turns),
        cost_usd: None,
    }
}

fn partial_command_stats(stdout: &str) -> AttemptStats {
    let Some(v) = last_json_object(stdout) else {
        return AttemptStats::default();
    };
    if v.get("schema_version").and_then(Value::as_str) != Some("bb.command_result.v1") {
        return AttemptStats::default();
    }
    AttemptStats {
        tokens_in: v.get("tokens_in").and_then(Value::as_i64),
        tokens_out: v.get("tokens_out").and_then(Value::as_i64),
        turns: v.get("turns").and_then(Value::as_i64),
        cost_usd: v.get("cost_usd").and_then(Value::as_f64),
    }
}

/// `opencode run --format json` emits one JSON object per line, each with a
/// `part` payload: `step-start`/`step-finish` bracket a model turn (cost and
/// per-turn token usage live on `step-finish`), `tool` records a completed
/// tool call, `text` carries assistant text. Unlike pi/omp there is no
/// streaming delta noise to filter — every line is a discrete, complete event.
fn opencode_step_finish_usage(stdout: &str) -> (i64, i64, i64, f64, bool) {
    let (mut turns, mut tokens_in, mut tokens_out) = (0i64, 0i64, 0i64);
    let mut cost = 0f64;
    let mut saw_usage = false;
    for line in stdout.lines() {
        let Ok(v) = serde_json::from_str::<Value>(line) else {
            continue;
        };
        let Some(part) = v.get("part") else { continue };
        if part.get("type").and_then(Value::as_str) != Some("step-finish") {
            continue;
        }
        turns += 1;
        cost += part.get("cost").and_then(Value::as_f64).unwrap_or(0.0);
        if let Some(tokens) = part.get("tokens") {
            saw_usage = true;
            tokens_in += tokens.get("input").and_then(Value::as_i64).unwrap_or(0);
            tokens_out += tokens.get("output").and_then(Value::as_i64).unwrap_or(0);
        }
    }
    (turns, tokens_in, tokens_out, cost, saw_usage)
}

fn partial_opencode_stats(stdout: &str) -> AttemptStats {
    let (turns, tokens_in, tokens_out, cost, saw_usage) = opencode_step_finish_usage(stdout);
    AttemptStats {
        tokens_in: saw_usage.then_some(tokens_in),
        tokens_out: saw_usage.then_some(tokens_out),
        turns: (turns > 0).then_some(turns),
        cost_usd: saw_usage.then_some(cost),
    }
}

fn parse_opencode(stdout: &str) -> Result<ParsedOutput> {
    let mut result = String::new();
    let mut saw_event = false;
    for line in stdout.lines() {
        let Ok(v) = serde_json::from_str::<Value>(line) else {
            continue;
        };
        saw_event = true;
        let Some(part) = v.get("part") else { continue };
        if part.get("type").and_then(Value::as_str) != Some("text") {
            continue;
        }
        if let Some(text) = part.get("text").and_then(Value::as_str) {
            if !text.trim().is_empty() {
                result = text.trim().to_string();
            }
        }
    }
    if !saw_event {
        bail!("opencode output: no JSONL events found");
    }
    if result.is_empty() {
        bail!("opencode output: no assistant text content — incomplete run");
    }
    let (turns, tokens_in, tokens_out, cost, saw_usage) = opencode_step_finish_usage(stdout);
    Ok(ParsedOutput {
        result,
        stats: AttemptStats {
            tokens_in: saw_usage.then_some(tokens_in),
            tokens_out: saw_usage.then_some(tokens_out),
            turns: (turns > 0).then_some(turns),
            cost_usd: saw_usage.then_some(cost),
        },
    })
}

fn parse_command(stdout: &str) -> Result<ParsedOutput> {
    let trimmed = stdout.trim().to_string();
    let Some(v) = last_json_object(stdout) else {
        return Ok(ParsedOutput {
            result: trimmed,
            stats: AttemptStats::default(),
        });
    };
    if v.get("schema_version").and_then(Value::as_str) != Some("bb.command_result.v1") {
        return Ok(ParsedOutput {
            result: trimmed,
            stats: AttemptStats::default(),
        });
    }
    let result = v
        .get("result")
        .and_then(Value::as_str)
        .unwrap_or("")
        .trim()
        .to_string();
    if result.is_empty() {
        bail!("command output: structured result has no non-empty 'result'");
    }
    Ok(ParsedOutput {
        result,
        stats: AttemptStats {
            tokens_in: v.get("tokens_in").and_then(Value::as_i64),
            tokens_out: v.get("tokens_out").and_then(Value::as_i64),
            turns: v.get("turns").and_then(Value::as_i64),
            cost_usd: v.get("cost_usd").and_then(Value::as_f64),
        },
    })
}
fn filtered_jsonl_command(inner: Vec<String>) -> Vec<String> {
    let quoted: Vec<String> = inner
        .iter()
        .map(|a| crate::substrate::local::shell_quote(a))
        .collect();
    vec![
        "sh".into(),
        "-c".into(),
        format!(
            "{{ {cmd}; echo $? > .bb-harness-exit; }} \
             | while IFS= read -r line; do \
                 case \"$line\" in *'\"type\":\"message_update\"'*|*'\"type\":\"message_start\"'*|*'\"role\":\"user\"'*|*'\"role\":\"toolResult\"'*|*'\"type\":\"tool_execution_update\"'*|*'\"type\":\"tool_execution_end\"'*|*'\"toolResults\":'*) ;; \
                   *) printf '%s\n' \"$line\" ;; \
                 esac; \
               done; \
             exit \"$(cat .bb-harness-exit)\"",
            cmd = quoted.join(" ")
        ),
        "sh".into(),
    ]
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
fn parse_agent_jsonl(harness: &str, stdout: &str) -> Result<ParsedOutput> {
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
        bail!("{harness} output: no JSONL events found");
    }
    let message = last_message
        .with_context(|| format!("{harness} output: no assistant message_end — incomplete run"))?;
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
        bail!("{harness} output: assistant message has no text content");
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
