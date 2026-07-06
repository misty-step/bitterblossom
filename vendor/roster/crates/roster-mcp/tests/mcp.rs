use serde_json::{Value, json};
use std::{
    io::Write,
    path::PathBuf,
    process::{Command, Stdio},
};

fn workspace_root() -> PathBuf {
    PathBuf::from(env!("CARGO_MANIFEST_DIR"))
        .parent()
        .and_then(|path| path.parent())
        .expect("workspace root")
        .to_path_buf()
}

#[test]
fn mcp_stdio_smoke_records_structured_success_and_error() {
    let mut child = Command::new(env!("CARGO_BIN_EXE_roster-mcp"))
        .stdin(Stdio::piped())
        .stdout(Stdio::piped())
        .stderr(Stdio::piped())
        .spawn()
        .expect("spawn roster-mcp");

    let mut stdin = child.stdin.take().expect("stdin");
    writeln!(stdin).expect("write blank line");
    writeln!(stdin, "not json").expect("write invalid json");
    for request in [
        json!({"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05"}}),
        json!({"jsonrpc":"2.0","id":2,"method":"tools/list"}),
        json!({"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"brief","arguments":{"root":workspace_root(),"agent":"orchestrator"}}}),
        json!({"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"show","arguments":{"root":workspace_root(),"agent":"missing"}}}),
        json!({"jsonrpc":"2.0","id":5,"method":"tools/call","params":{"name":"materialize","arguments":{"root":workspace_root(),"agent":"cerberus","harness":"codex"}}}),
    ] {
        writeln!(stdin, "{request}").expect("write request");
    }
    drop(stdin);

    let output = child.wait_with_output().expect("wait roster-mcp");
    assert!(output.status.success());
    let stderr = String::from_utf8(output.stderr).expect("utf8 stderr");
    assert!(stderr.contains("roster-mcp: invalid json"));
    let stdout = String::from_utf8(output.stdout).expect("utf8 stdout");
    let responses = stdout
        .lines()
        .map(|line| serde_json::from_str::<Value>(line).expect("json response"))
        .collect::<Vec<_>>();

    assert_eq!(responses.len(), 5);
    assert_eq!(
        responses[0]["result"]["serverInfo"]["name"].as_str(),
        Some("roster")
    );
    assert_eq!(responses[1]["result"]["tools"][0]["name"], "list");
    assert!(
        responses[2]["result"]["content"][0]["text"]
            .as_str()
            .unwrap()
            .contains("# Roster Brief: orchestrator")
    );
    assert_eq!(
        responses[2]["result"]["structuredContent"]["agent"],
        "orchestrator"
    );
    assert_eq!(responses[3]["result"]["isError"], true);
    assert!(
        responses[3]["result"]["content"][0]["text"]
            .as_str()
            .unwrap()
            .contains("unknown agent")
    );
    assert!(
        responses[4]["result"]["content"][0]["text"]
            .as_str()
            .unwrap()
            .contains("# Roster Brief: cerberus")
    );
    assert_eq!(
        responses[4]["result"]["structuredContent"]["harness"],
        "codex"
    );
}
