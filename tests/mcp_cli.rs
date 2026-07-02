//! Read-only MCP stdio server smoke test (backlog 078). Drives `bb mcp serve`
//! as a subprocess over stdin/stdout, proving the JSON-RPC contract:
//! `initialize`, `tools/list`, focused `tools/call` for the read-only tools,
//! rejection of an unknown (would-be mutating) tool name, and notification
//! silence. Tool outputs are compared to the same shapes `bb ... --json`
//! produces — MCP is an adapter, not a second implementation.

use std::fs;
use std::io::{Read, Write};
use std::process::{Command, Stdio};

use serde_json::{json, Value};

fn write_plane(root: &std::path::Path) {
    fs::create_dir_all(root.join("agents")).unwrap();
    fs::create_dir_all(root.join("tasks/demo")).unwrap();
    fs::write(
        root.join("agents/a.toml"),
        "version = 1\nharness = \"command\"\nmodel = \"\"\nbin = \"/usr/bin/true\"\n",
    )
    .unwrap();
    fs::write(root.join("tasks/demo/card.md"), "card\n").unwrap();
    fs::write(
        root.join("tasks/demo/task.toml"),
        "agent = \"a\"\nsubstrate = \"local\"\n[[trigger]]\nkind = \"manual\"\n",
    )
    .unwrap();
    fs::write(root.join("plane.toml"), "dev = true\n").unwrap();
}

/// One JSON-RPC line for the server.
fn req(id: Option<i64>, method: &str, params: Option<Value>) -> String {
    let mut v = json!({ "jsonrpc": "2.0", "method": method });
    if let Some(id) = id {
        v["id"] = json!(id);
    }
    if let Some(p) = params {
        v["params"] = p;
    }
    serde_json::to_string(&v).unwrap()
}

/// Read one newline-delimited JSON response from the server stdout.
fn read_response(out: &mut std::process::ChildStdout) -> Value {
    let mut buf = Vec::new();
    let mut byte = [0u8; 1];
    loop {
        match out.read(&mut byte) {
            Ok(0) => break,
            Ok(_) => {
                if byte[0] == b'\n' {
                    break;
                }
                buf.push(byte[0]);
            }
            Err(e) => panic!("read response: {e}"),
        }
    }
    let text = String::from_utf8(buf).unwrap();
    serde_json::from_str(&text).unwrap_or_else(|e| panic!("parse response {text:?}: {e}"))
}

#[test]
fn mcp_serve_read_only_tools_list_and_call() {
    let dir = tempfile::tempdir().unwrap();
    write_plane(dir.path());
    let root = dir.path().to_str().unwrap();

    // Seed one run so bb_status has real ledger content to report.
    assert!(
        Command::new(env!("CARGO_BIN_EXE_bb"))
            .args([
                "--config",
                root,
                "run",
                "demo",
                "--payload",
                "{\"ok\":true}",
                "--json"
            ])
            .stdout(Stdio::null())
            .stderr(Stdio::null())
            .status()
            .unwrap()
            .success(),
        "seed run failed"
    );

    let mut child = Command::new(env!("CARGO_BIN_EXE_bb"))
        .args(["--config", root, "mcp", "serve"])
        .stdin(Stdio::piped())
        .stdout(Stdio::piped())
        .stderr(Stdio::piped())
        .spawn()
        .unwrap();
    let mut stdin = child.stdin.take().unwrap();
    let mut stdout = child.stdout.take().unwrap();

    // initialize (request) -> capabilities advertise tools.
    writeln!(stdin, "{}", req(Some(1), "initialize", None)).unwrap();
    let init = read_response(&mut stdout);
    assert_eq!(init["jsonrpc"], "2.0");
    assert_eq!(init["id"], 1);
    assert_eq!(init["result"]["protocolVersion"], "2024-11-05");
    assert!(init["result"]["capabilities"]["tools"].is_object());
    assert_eq!(init["result"]["serverInfo"]["name"], "bitterblossom");

    // notifications/initialized -> no response (silent).
    writeln!(stdin, "{}", req(None, "notifications/initialized", None)).unwrap();

    // tools/list -> exact read-only tool names present for this slice.
    writeln!(stdin, "{}", req(Some(2), "tools/list", None)).unwrap();
    let list = read_response(&mut stdout);
    let tools = list["result"]["tools"].as_array().unwrap();
    let names: Vec<&str> = tools.iter().map(|t| t["name"].as_str().unwrap()).collect();
    assert_eq!(
        names,
        vec![
            "bb_status",
            "bb_check",
            "bb_tasks",
            "bb_dlq_list",
            "bb_preflight"
        ]
    );
    for t in tools {
        assert_eq!(t["inputSchema"]["type"], "object");
    }

    // tools/call bb_status -> succeeds, output matches `bb status --json`.
    writeln!(
        stdin,
        "{}",
        req(Some(3), "tools/call", Some(json!({ "name": "bb_status" })))
    )
    .unwrap();
    let call = read_response(&mut stdout);
    assert_eq!(call["id"], 3);
    assert_eq!(call["result"]["isError"], false);
    let status_text = call["result"]["content"][0]["text"].as_str().unwrap();
    let status: Value = serde_json::from_str(status_text).unwrap();
    assert_eq!(status["summary"]["open_dlq"], 0);
    assert!(status["tasks"]
        .as_array()
        .unwrap()
        .iter()
        .any(|t| t["task"] == "demo"));

    // Cross-check: the MCP bb_status shape equals `bb status --json` modulo
    // the volatile per-call `generated_at` timestamp.
    let cli = Command::new(env!("CARGO_BIN_EXE_bb"))
        .args(["--config", root, "status", "--json"])
        .output()
        .unwrap();
    assert!(cli.status.success());
    let mut cli_status: Value = serde_json::from_slice(&cli.stdout).unwrap();
    let mut status_cmp = status.clone();
    status_cmp["generated_at"] = json!(null);
    cli_status["generated_at"] = json!(null);
    assert_eq!(
        status_cmp, cli_status,
        "MCP bb_status shape drifted from `bb status --json`"
    );

    // tools/call bb_check -> succeeds, output matches `bb check --json`.
    writeln!(
        stdin,
        "{}",
        req(Some(4), "tools/call", Some(json!({ "name": "bb_check" })))
    )
    .unwrap();
    let check_call = read_response(&mut stdout);
    assert_eq!(check_call["result"]["isError"], false);
    let check_text = check_call["result"]["content"][0]["text"].as_str().unwrap();
    let check: Value = serde_json::from_str(check_text).unwrap();
    assert!(check["tasks"]
        .as_array()
        .unwrap()
        .iter()
        .any(|t| t == "demo"));
    let cli_check = Command::new(env!("CARGO_BIN_EXE_bb"))
        .args(["--config", root, "check", "--json"])
        .output()
        .unwrap();
    assert!(cli_check.status.success());
    let cli_check_val: Value = serde_json::from_slice(&cli_check.stdout).unwrap();
    assert_eq!(
        check, cli_check_val,
        "MCP bb_check shape drifted from `bb check --json`"
    );

    // tools/call bb_tasks -> same shape as `bb task list --json`.
    writeln!(
        stdin,
        "{}",
        req(Some(5), "tools/call", Some(json!({ "name": "bb_tasks" })))
    )
    .unwrap();
    let tasks_call = read_response(&mut stdout);
    assert_eq!(tasks_call["result"]["isError"], false);
    let tasks_text = tasks_call["result"]["content"][0]["text"].as_str().unwrap();
    let tasks: Value = serde_json::from_str(tasks_text).unwrap();
    let cli_tasks = Command::new(env!("CARGO_BIN_EXE_bb"))
        .args(["--config", root, "task", "list", "--json"])
        .output()
        .unwrap();
    assert!(cli_tasks.status.success());
    assert_eq!(
        tasks,
        serde_json::from_slice::<Value>(&cli_tasks.stdout).unwrap(),
        "MCP bb_tasks shape drifted from `bb task list --json`"
    );

    // tools/call bb_dlq_list -> same shape as `bb dlq list --json`.
    writeln!(
        stdin,
        "{}",
        req(
            Some(6),
            "tools/call",
            Some(json!({ "name": "bb_dlq_list" }))
        )
    )
    .unwrap();
    let dlq_call = read_response(&mut stdout);
    assert_eq!(dlq_call["result"]["isError"], false);
    let dlq_text = dlq_call["result"]["content"][0]["text"].as_str().unwrap();
    let dlq: Value = serde_json::from_str(dlq_text).unwrap();
    let cli_dlq = Command::new(env!("CARGO_BIN_EXE_bb"))
        .args(["--config", root, "dlq", "list", "--json"])
        .output()
        .unwrap();
    assert!(cli_dlq.status.success());
    assert_eq!(
        dlq,
        serde_json::from_slice::<Value>(&cli_dlq.stdout).unwrap(),
        "MCP bb_dlq_list shape drifted from `bb dlq list --json`"
    );

    // tools/call bb_preflight -> same shape as `bb preflight <task> --json`.
    writeln!(
        stdin,
        "{}",
        req(
            Some(7),
            "tools/call",
            Some(json!({ "name": "bb_preflight", "arguments": { "task": "demo" } }))
        )
    )
    .unwrap();
    let preflight_call = read_response(&mut stdout);
    assert_eq!(preflight_call["result"]["isError"], false);
    let preflight_text = preflight_call["result"]["content"][0]["text"]
        .as_str()
        .unwrap();
    let preflight: Value = serde_json::from_str(preflight_text).unwrap();
    let cli_preflight = Command::new(env!("CARGO_BIN_EXE_bb"))
        .args(["--config", root, "preflight", "demo", "--json"])
        .output()
        .unwrap();
    assert!(cli_preflight.status.success());
    assert_eq!(
        preflight,
        serde_json::from_slice::<Value>(&cli_preflight.stdout).unwrap(),
        "MCP bb_preflight shape drifted from `bb preflight demo --json`"
    );

    // Unknown / would-be mutating tool is rejected (read-only by construction).
    writeln!(
        stdin,
        "{}",
        req(
            Some(8),
            "tools/call",
            Some(json!({ "name": "bb_runs_cancel" }))
        )
    )
    .unwrap();
    let rejected = read_response(&mut stdout);
    assert!(
        rejected.get("error").is_some(),
        "expected JSON-RPC error for unknown tool"
    );
    assert_eq!(rejected["error"]["code"], -32602);
    assert!(rejected["error"]["message"]
        .as_str()
        .unwrap()
        .contains("unknown tool: bb_runs_cancel"));

    drop(stdin);
    let status = child.wait().unwrap();
    assert!(status.success(), "mcp serve exited {status}");
}
