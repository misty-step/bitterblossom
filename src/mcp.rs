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
    call: fn(&Plane, &Ledger) -> Result<Value>,
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
            call: crate::health::status_view,
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
            call: serve::check_view,
        },
    ]
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
                    let result =
                        Ledger::open(&plane.db_path()).and_then(|ledger| (t.call)(plane, &ledger));
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
