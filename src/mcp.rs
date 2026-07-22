//! MCP (Model Context Protocol) stdio adapter over the canonical
//! Bitterblossom view helpers.
//!
//! `bb mcp serve` speaks JSON-RPC 2.0 over stdio (newline-delimited). It
//! exposes a fixed table of read-only tools whose outputs come from the same
//! helpers the CLI and HTTP API use — MCP never builds its own status/check
//! shapes, so output stays compatible with `bb ... --json`. Read-only is the
//! default and unconditional posture (backlog 078): those tools are always
//! registered and no environment variable weakens that.
//!
//! `bb_dispatch` (bitterblossom-116) is the one mutating exception, and it is
//! opt-in by construction: it exists in `tools/list` and answers `tools/call`
//! only when `BB_MCP_ENABLE_DISPATCH` is set on the server process. It is
//! handled entirely outside the shared read-only `Tool` table below (see
//! `dispatch_tool_descriptor`/`call_dispatch_tool`) so the read-only slice's
//! code path never has a mutating branch to accidentally enable. It enqueues
//! the same `bb.dispatch_job.v1` payload `bb dispatch` builds and never
//! merges, deploys, or otherwise reaches past `Ledger::ingest` -- see
//! `docs/mcp-dispatch-authority.md` for the authority boundary.

use std::io::{BufRead, Write};

use anyhow::{Context, Result};
use serde_json::{json, Value};

use crate::artifacts;
use crate::dispatch;
use crate::ledger::{IngressRequest, Ledger};
use crate::serve;
use crate::spec::Plane;

/// MCP protocol version advertised on `initialize`. `2024-11-05` is the
/// stable baseline every consuming client supports.
const PROTOCOL_VERSION: &str = "2024-11-05";

/// Name of the opt-in mutating dispatch tool. Grep-able by design: this is
/// the one MCP tool name handled outside the read-only `Tool` table.
const DISPATCH_TOOL_NAME: &str = "bb_dispatch";

/// The one env var that turns `bb_dispatch` on. Explicit and grep-able by
/// design -- an operator or a `grep BB_MCP_ENABLE_DISPATCH` audit can see at
/// a glance whether a given `bb mcp serve` process can mutate anything.
const DISPATCH_ENABLE_ENV: &str = "BB_MCP_ENABLE_DISPATCH";

/// Whether the mutating dispatch tool is enabled for this process. Checked
/// fresh on every `tools/list`/`tools/call` so toggling the env var and
/// restarting `bb mcp serve` is the entire enablement story -- no separate
/// config file, no runtime toggle RPC.
fn dispatch_enabled() -> bool {
    matches!(
        std::env::var(DISPATCH_ENABLE_ENV).ok().as_deref(),
        Some("1") | Some("true")
    )
}

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
            name: "bb_artifacts_list",
            description: "List top-level artifact files across a run's attempts, \
                including size, content type, and binary metadata. Read-only. Same \
                shape as `bb artifacts list <run-id> --json`.",
            input_schema: json!({
                "type": "object",
                "required": ["run_id"],
                "properties": {
                    "run_id": { "type": "string" }
                },
                "additionalProperties": false,
            }),
            call: artifacts_list_tool,
        },
        Tool {
            name: "bb_artifact_read",
            description: "Read one safe text/JSON artifact from a run's attempts. \
                Binary, oversized, missing, and unsafe paths are returned or rejected \
                by the same contract as `bb artifacts read <run-id> <path> --json`.",
            input_schema: json!({
                "type": "object",
                "required": ["run_id", "path"],
                "properties": {
                    "run_id": { "type": "string" },
                    "path": { "type": "string" }
                },
                "additionalProperties": false,
            }),
            call: artifact_read_tool,
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

fn artifacts_list_tool(_: &Plane, ledger: &Ledger, args: &Value) -> Result<Value> {
    let run_id = required_string_arg(args, "run_id")?;
    Ok(serde_json::to_value(artifacts::list(ledger, run_id)?)?)
}

fn artifact_read_tool(_: &Plane, ledger: &Ledger, args: &Value) -> Result<Value> {
    let run_id = required_string_arg(args, "run_id")?;
    let path = required_string_arg(args, "path")?;
    Ok(serde_json::to_value(artifacts::read(
        ledger, run_id, path,
    )?)?)
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

/// Descriptor for the opt-in `bb_dispatch` tool. Only added to `tools/list`
/// when `dispatch_enabled()` is true -- see the module doc for why this is
/// kept structurally separate from the read-only `Tool` table.
fn dispatch_tool_descriptor() -> Value {
    json!({
        "name": DISPATCH_TOOL_NAME,
        "description": "MUTATING. Opt-in only (requires BB_MCP_ENABLE_DISPATCH=1 on the \
            server process). Enqueues one bounded ad hoc dispatch job -- the identical \
            bb.dispatch_job.v1 payload `bb dispatch` builds -- and returns immediately with \
            the run id plus follow-up bb logs/runs/artifacts commands; a running `bb serve` \
            drains the run asynchronously. This tool never merges, deploys, or mutates \
            anything beyond enqueuing that one run: it is exactly as authoritative as \
            `bb dispatch` and no more. A repeat call with the same repo/label/branch_slug/ \
            base_ref is refused (returns the original run id, duplicate: true) unless \
            force: true is set. See docs/mcp-dispatch-authority.md.",
        "inputSchema": {
            "type": "object",
            "required": ["repo", "prompt"],
            "properties": {
                "repo": {
                    "type": "string",
                    "description": "Local directory path to the repo, same as `bb dispatch --repo`."
                },
                "prompt": {
                    "type": "string",
                    "description": "The dispatch brief text, inline (no file staging needed). \
                        Capped at the same size `bb dispatch --brief` enforces."
                },
                "model": {
                    "type": "string",
                    "description": "Optional per-run model override, same as `bb dispatch --model`."
                },
                "label": {
                    "type": "string",
                    "description": "Optional human-readable label, same as `bb dispatch --label`. \
                        Defaults to \"dispatch\" when omitted."
                },
                "branch_slug": {
                    "type": "string",
                    "description": "Optional explicit branch slug; defaults to a slugified `label`."
                },
                "base_ref": {
                    "type": "string",
                    "description": "Optional base git ref for the dispatched agent's own repo \
                        setup; carried through the payload as context, same as the `base_ref` \
                        field scripts/bb-dispatch-build already writes."
                },
                "force": {
                    "type": "boolean",
                    "default": false,
                    "description": "Bypass duplicate refusal and always mint a fresh run, even \
                        if an identical (repo, label, branch_slug, base_ref) dispatch already \
                        exists."
                }
            },
            "additionalProperties": false
        }
    })
}

fn call_dispatch_tool(plane: &Plane, id: Value, args: &Value) -> Value {
    match run_dispatch_tool(plane, args) {
        Ok(value) => {
            let text = serde_json::to_string_pretty(&value).unwrap_or_else(|_| value.to_string());
            json!({
                "jsonrpc": "2.0",
                "id": id,
                "result": {
                    "content": [{ "type": "text", "text": text }],
                    "isError": false,
                },
            })
        }
        Err(e) => call_error(id, &format!("{e:#}")),
    }
}

/// Validate inputs, build the shared `bb.dispatch_job.v1` payload, derive the
/// default duplicate-refusal idempotency key (skipped when `force: true`),
/// and enqueue through the exact same `Ledger::ingest` path every other
/// trigger (webhook, cron, CLI `bb dispatch`/`bb run`) already uses. No new
/// mutation capability is introduced here -- this is the same ingress door,
/// opened to one more caller.
fn run_dispatch_tool(plane: &Plane, args: &Value) -> Result<Value> {
    let repo = required_string_arg(args, "repo")?;
    let prompt = required_string_arg(args, "prompt")?;
    let model = string_arg(args, "model").map(str::to_string);
    let label = string_arg(args, "label")
        .filter(|v| !v.is_empty())
        .unwrap_or("dispatch")
        .to_string();
    let branch_slug = string_arg(args, "branch_slug")
        .filter(|v| !v.is_empty())
        .map(dispatch::slugify_label)
        .unwrap_or_else(|| dispatch::slugify_label(&label));
    let base_ref = string_arg(args, "base_ref")
        .filter(|v| !v.is_empty())
        .map(str::to_string);
    let force = args.get("force").and_then(Value::as_bool).unwrap_or(false);

    let repo_path = std::path::Path::new(repo);
    let payload = dispatch::build_dispatch_job_payload(
        repo_path,
        prompt,
        model,
        label.clone(),
        branch_slug.clone(),
        base_ref.clone(),
    )?;
    // Re-derive the canonical repo string for the idempotency key from the
    // same payload we just built rather than canonicalizing twice with two
    // chances to disagree.
    let payload_value: Value = serde_json::from_str(&payload)?;
    let canon_repo = payload_value["repo"]
        .as_str()
        .context("dispatch payload missing repo")?;

    let task = dispatch::default_dispatch_task(plane)?;
    let idempotency_key = (!force).then(|| {
        dispatch::dispatch_idempotency_key(canon_repo, &label, &branch_slug, base_ref.as_deref())
    });

    let mut ledger = Ledger::open(&plane.db_path())?;
    let outcome = crate::workflow_service::WorkflowService::dispatch_for(
        plane,
        &mut ledger,
        crate::workflow_service::auth_context_for_local()?,
        IngressRequest {
            task: &task,
            trigger_kind: "manual",
            idempotency_key: idempotency_key.as_deref(),
            source_event_id: None,
            payload: Some(&payload),
            parent_run_id: None,
        },
    )?;

    Ok(json!({
        "run_id": outcome.run_id,
        "task": task,
        "state": outcome.state,
        "duplicate": outcome.duplicate,
        "idempotency_key": idempotency_key,
        "follow_up": {
            "logs": format!("bb logs -f {}", outcome.run_id),
            "runs_show": format!("bb runs show {} --json", outcome.run_id),
            "artifacts_list": format!("bb artifacts list {} --json", outcome.run_id),
        },
    }))
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
            Ok(req) => route_request(plane, &req),
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

/// Route one parsed JSON-RPC request. Returns `None` for notifications
/// (no `id`) so the server stays silent, matching the JSON-RPC contract.
fn route_request(plane: &Plane, req: &Value) -> Option<Value> {
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
            let mut tools = table
                .iter()
                .map(|t| {
                    json!({
                        "name": t.name,
                        "description": t.description,
                        "inputSchema": t.input_schema,
                    })
                })
                .collect::<Vec<_>>();
            if dispatch_enabled() {
                tools.push(dispatch_tool_descriptor());
            }
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
            if name == DISPATCH_TOOL_NAME {
                if !dispatch_enabled() {
                    return Some(error_response(
                        id,
                        -32602,
                        &format!(
                            "unknown tool: {DISPATCH_TOOL_NAME} (opt-in mutating dispatch is \
                             disabled; set {DISPATCH_ENABLE_ENV}=1 on the `bb mcp serve` process \
                             to enable it)"
                        ),
                    ));
                }
                let args = params
                    .and_then(|p| p.get("arguments"))
                    .unwrap_or(&Value::Null);
                return Some(call_dispatch_tool(plane, id, args));
            }
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
