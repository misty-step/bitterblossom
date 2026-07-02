//! Read-only MCP (Model Context Protocol) stdio adapter over the canonical
//! Bitterblossom view helpers.
//!
//! `bb mcp serve` speaks JSON-RPC 2.0 over stdio (newline-delimited). It
//! exposes a fixed table of read-only tools whose outputs come from the same
//! helpers the CLI and HTTP API use — MCP never builds its own status/check
//! shapes, so output stays compatible with `bb ... --json`. No mutating
//! tools are registered; this slice is read-only by construction (backlog
//! 078). A future writable MCP surface requires its own backlog item and
//! the graduation signals recorded there.

use std::io::{BufRead, Write};

use anyhow::Result;
use serde_json::{json, Value};

use crate::ledger::Ledger;
use crate::serve;
use crate::spec::Plane;

/// MCP protocol version advertised on `initialize`. `2024-11-05` is the
/// stable baseline every consuming client supports.
const PROTOCOL_VERSION: &str = "2024-11-05";

/// A read-only tool backed by a shared view helper. The helper takes a
/// freshly opened ledger per call so a long-lived server sees current state.
struct Tool {
    name: &'static str,
    description: &'static str,
    input_schema: Value,
    call: fn(&Plane, &Ledger, &Value) -> Result<Value>,
}

fn tools() -> Vec<Tool> {
    vec![
        Tool {
            name: "bb_status",
            description: "Operator truth: tasks, runs by state, parked tasks, open \
                DLQs, and the safe next action for each. Read-only. Same shape as \
                `bb status --json` and `GET /api/status`.",
            input_schema: json!({
                "type": "object",
                "properties": {},
                "additionalProperties": false,
            }),
            call: status_tool,
        },
        Tool {
            name: "bb_check",
            description: "Validate the config surface: loaded agents, tasks, \
                substrates, budget caps, and the task view. Read-only. Same shape \
                as `bb check --json`.",
            input_schema: json!({
                "type": "object",
                "properties": {},
                "additionalProperties": false,
            }),
            call: check_tool,
        },
        Tool {
            name: "bb_tasks",
            description: "List tasks with agent, substrate, triggers, verdict, \
                parked state, budget caps, and policy. Read-only. Same shape as \
                `bb task list --json` and `GET /api/tasks`.",
            input_schema: empty_schema(),
            call: tasks_tool,
        },
        Tool {
            name: "bb_runs_list",
            description: "List recent runs, optionally filtered by task or state. \
                Read-only. Same shape as `bb runs list --json` and `GET /api/runs`.",
            input_schema: json!({
                "type": "object",
                "properties": {
                    "task": { "type": "string" },
                    "state": { "type": "string" }
                },
                "additionalProperties": false,
            }),
            call: runs_list_tool,
        },
        Tool {
            name: "bb_runs_show",
            description: "Show one run bundle with attempts, events, and progress \
                classification. Read-only. Same shape as `bb runs show --json` and \
                `GET /api/runs/<id>`.",
            input_schema: json!({
                "type": "object",
                "required": ["run_id"],
                "properties": {
                    "run_id": { "type": "string" }
                },
                "additionalProperties": false,
            }),
            call: runs_show_tool,
        },
        Tool {
            name: "bb_dlq_list",
            description: "List dead letters with replay/acknowledgement status. \
                Read-only. Same shape as `bb dlq list --json` and `GET /api/dlq`.",
            input_schema: empty_schema(),
            call: dlq_list_tool,
        },
        Tool {
            name: "bb_preflight",
            description: "Run read-only pre-dispatch checks for one task or the \
                submission-storm member set. Same shape as `bb preflight --json`.",
            input_schema: json!({
                "type": "object",
                "properties": {
                    "task": { "type": "string" },
                    "storm": { "type": "boolean", "default": false }
                },
                "additionalProperties": false,
            }),
            call: preflight_tool,
        },
        Tool {
            name: "bb_gate",
            description: "Evaluate the submission gate by submission id or change \
                key. Read-only. Same shape as `bb gate --json` and `GET /api/gate`.",
            input_schema: json!({
                "type": "object",
                "properties": {
                    "submission": { "type": "string" },
                    "change": { "type": "string" }
                },
                "additionalProperties": false,
            }),
            call: gate_tool,
        },
    ]
}

fn empty_schema() -> Value {
    json!({ "type": "object", "properties": {}, "additionalProperties": false })
}

fn status_tool(plane: &Plane, ledger: &Ledger, _: &Value) -> Result<Value> {
    crate::health::status_view(plane, ledger)
}

fn check_tool(plane: &Plane, ledger: &Ledger, _: &Value) -> Result<Value> {
    serve::check_view(plane, ledger)
}

fn tasks_tool(plane: &Plane, ledger: &Ledger, _: &Value) -> Result<Value> {
    Ok(Value::Array(serve::tasks_view(plane, ledger)?))
}

fn runs_list_tool(_: &Plane, ledger: &Ledger, args: &Value) -> Result<Value> {
    serve::runs_view(ledger, string_arg(args, "task"), string_arg(args, "state"))
}

fn runs_show_tool(_: &Plane, ledger: &Ledger, args: &Value) -> Result<Value> {
    let run_id = required_string_arg(args, "run_id")?;
    serve::run_view(ledger, run_id)
}

fn dlq_list_tool(_: &Plane, ledger: &Ledger, _: &Value) -> Result<Value> {
    Ok(serde_json::to_value(ledger.list_dead_letters()?)?)
}

fn preflight_tool(plane: &Plane, _: &Ledger, args: &Value) -> Result<Value> {
    let task = args.get("task").and_then(Value::as_str);
    let storm = args.get("storm").and_then(Value::as_bool).unwrap_or(false);
    Ok(serde_json::to_value(crate::preflight::run(
        plane, task, storm,
    )?)?)
}

fn gate_tool(plane: &Plane, ledger: &Ledger, args: &Value) -> Result<Value> {
    Ok(serde_json::to_value(serve::gate_view(
        plane,
        ledger,
        string_arg(args, "submission"),
        string_arg(args, "change"),
    )?)?)
}

fn string_arg<'a>(args: &'a Value, name: &str) -> Option<&'a str> {
    args.get(name).and_then(Value::as_str)
}

fn required_string_arg<'a>(args: &'a Value, name: &str) -> Result<&'a str> {
    string_arg(args, name)
        .filter(|v| !v.is_empty())
        .ok_or_else(|| {
            anyhow::anyhow!("argument '{name}' is required and must be a non-empty string")
        })
}
/// Run the read-only MCP stdio server. Reads newline-delimited JSON-RPC from
/// stdin and writes one response per request to stdout. No network listener.
pub fn serve_stdio(plane: &Plane) -> Result<()> {
    let stdin = std::io::stdin();
    let stdout = std::io::stdout();
    let mut out = stdout.lock();
    for line in stdin.lock().lines() {
        let line = line?;
        if line.trim().is_empty() {
            continue;
        }
        let response = match serde_json::from_str::<Value>(&line) {
            Ok(req) => dispatch(plane, &req),
            Err(e) => Some(error_response(
                Value::Null,
                -32700,
                &format!("parse error: {e}"),
            )),
        };
        if let Some(resp) = response {
            writeln!(out, "{resp}")?;
            out.flush()?;
        }
    }
    Ok(())
}

/// Dispatch one parsed JSON-RPC request. Returns `None` for notifications
/// (no `id`) so the server stays silent, matching the JSON-RPC contract.
fn dispatch(plane: &Plane, req: &Value) -> Option<Value> {
    let id = req.get("id").cloned();
    let method = req.get("method").and_then(|m| m.as_str()).unwrap_or("");
    let params = req.get("params");

    match method {
        "notifications/initialized" => None,
        "initialize" => {
            let id = id.unwrap_or(Value::Null);
            Some(json!({
                "jsonrpc": "2.0",
                "id": id,
                "result": {
                    "protocolVersion": PROTOCOL_VERSION,
                    "capabilities": { "tools": {} },
                    "serverInfo": {
                        "name": "bitterblossom",
                        "version": env!("CARGO_PKG_VERSION"),
                    },
                },
            }))
        }
        "tools/list" => {
            let id = id.unwrap_or(Value::Null);
            let table = tools();
            let tools = table
                .iter()
                .map(|t| {
                    json!({
                        "name": t.name,
                        "description": t.description,
                        "inputSchema": t.input_schema,
                    })
                })
                .collect::<Vec<_>>();
            Some(json!({
                "jsonrpc": "2.0",
                "id": id,
                "result": { "tools": tools },
            }))
        }
        "tools/call" => {
            let id = id.unwrap_or(Value::Null);
            let name = params
                .and_then(|p| p.get("name"))
                .and_then(|n| n.as_str())
                .unwrap_or("");
            let table = tools();
            match table.iter().find(|t| t.name == name) {
                Some(t) => {
                    let args = params
                        .and_then(|p| p.get("arguments"))
                        .unwrap_or(&Value::Null);
                    let result = Ledger::open(&plane.db_path())
                        .and_then(|ledger| (t.call)(plane, &ledger, args));
                    match result {
                        Ok(value) => {
                            let text = serde_json::to_string_pretty(&value)
                                .unwrap_or_else(|_| value.to_string());
                            Some(json!({
                                "jsonrpc": "2.0",
                                "id": id,
                                "result": {
                                    "content": [{ "type": "text", "text": text }],
                                    "isError": false,
                                },
                            }))
                        }
                        Err(e) => Some(call_error(id, &format!("{e:#}"))),
                    }
                }
                None => Some(error_response(
                    id,
                    -32602,
                    &format!(
                        "unknown tool: {name} (only read-only tools are registered: {})",
                        table.iter().map(|t| t.name).collect::<Vec<_>>().join(", ")
                    ),
                )),
            }
        }
        _ => id.map(|id| error_response(id, -32601, &format!("method not found: {method}"))),
    }
}

fn error_response(id: Value, code: i32, message: &str) -> Value {
    json!({
        "jsonrpc": "2.0",
        "id": id,
        "error": { "code": code, "message": message },
    })
}

/// A tool that ran but failed returns `isError: true` with the error text as
/// content, per the MCP `tools/call` error convention (distinct from a
/// JSON-RPC protocol error).
fn call_error(id: Value, message: &str) -> Value {
    json!({
        "jsonrpc": "2.0",
        "id": id,
        "result": {
            "content": [{ "type": "text", "text": message }],
            "isError": true,
        },
    })
}
