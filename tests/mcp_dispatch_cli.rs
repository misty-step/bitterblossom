//! Opt-in mutating MCP dispatch tool (bitterblossom-116): `bb_dispatch`.
//!
//! Proves, over the same JSON-RPC stdio contract `tests/mcp_cli.rs` exercises
//! for the read-only tools:
//! - the default read-only posture is unchanged: `bb_dispatch` is absent from
//!   `tools/list` and `tools/call` refuses it when `BB_MCP_ENABLE_DISPATCH`
//!   is unset;
//! - enabling it exposes the tool and it rejects unsafe/missing inputs
//!   before ever touching the ledger;
//! - a successful call enqueues the identical `bb.dispatch_job.v1` payload
//!   the CLI `bb dispatch` command builds;
//! - a repeat dispatch of the same (repo, label, branch_slug, base_ref) is
//!   refused (same run id, `duplicate: true`) unless `force: true` is set.

use std::fs;
use std::io::{Read, Write};
use std::os::unix::fs::PermissionsExt;
use std::process::{Child, ChildStdin, ChildStdout, Command, Stdio};

use bitterblossom::ledger::Ledger;
use bitterblossom::spec::Plane;
use serde_json::{json, Value};

fn write_dispatch_plane(root: &std::path::Path) {
    fs::create_dir_all(root.join("agents")).unwrap();
    fs::create_dir_all(root.join("tasks/dispatch")).unwrap();
    fs::write(root.join("plane.toml"), "dev = true\n").unwrap();
    let stub = root.join("dispatch-agent.sh");
    fs::write(
        &stub,
        "#!/bin/sh\ncat > /dev/null\nprintf '{}\\n' > REPORT.json\n",
    )
    .unwrap();
    fs::set_permissions(&stub, fs::Permissions::from_mode(0o755)).unwrap();
    fs::write(
        root.join("agents/stub.toml"),
        format!(
            "version = 1\nharness = \"command\"\nmodel = \"\"\nbin = \"{}\"\n",
            stub.display()
        ),
    )
    .unwrap();
    fs::write(root.join("tasks/dispatch/card.md"), "dispatch card\n").unwrap();
    fs::write(
        root.join("tasks/dispatch/task.toml"),
        "agent = \"stub\"\nsubstrate = \"local\"\n[[trigger]]\nkind = \"manual\"\n",
    )
    .unwrap();
}

fn req(id: i64, method: &str, params: Option<Value>) -> String {
    let mut v = json!({ "jsonrpc": "2.0", "id": id, "method": method });
    if let Some(p) = params {
        v["params"] = p;
    }
    serde_json::to_string(&v).unwrap()
}

fn read_response(out: &mut ChildStdout) -> Value {
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

fn spawn_mcp_server(root: &str, dispatch_enabled: bool) -> Child {
    let mut cmd = Command::new(env!("CARGO_BIN_EXE_bb"));
    cmd.args(["--config", root, "mcp", "serve"])
        .stdin(Stdio::piped())
        .stdout(Stdio::piped())
        .stderr(Stdio::piped());
    if dispatch_enabled {
        cmd.env("BB_MCP_ENABLE_DISPATCH", "1");
    } else {
        cmd.env_remove("BB_MCP_ENABLE_DISPATCH");
    }
    cmd.spawn().unwrap()
}

fn call(
    stdin: &mut ChildStdin,
    stdout: &mut ChildStdout,
    id: i64,
    name: &str,
    arguments: Value,
) -> Value {
    writeln!(
        stdin,
        "{}",
        req(
            id,
            "tools/call",
            Some(json!({ "name": name, "arguments": arguments }))
        )
    )
    .unwrap();
    read_response(stdout)
}

fn tool_names(stdin: &mut ChildStdin, stdout: &mut ChildStdout, id: i64) -> Vec<String> {
    writeln!(stdin, "{}", req(id, "tools/list", None)).unwrap();
    let list = read_response(stdout);
    list["result"]["tools"]
        .as_array()
        .unwrap()
        .iter()
        .map(|t| t["name"].as_str().unwrap().to_string())
        .collect()
}

#[test]
fn bb_dispatch_is_absent_by_default() {
    let dir = tempfile::tempdir().unwrap();
    write_dispatch_plane(dir.path());
    let root = dir.path().to_str().unwrap();

    let mut child = spawn_mcp_server(root, false);
    let mut stdin = child.stdin.take().unwrap();
    let mut stdout = child.stdout.take().unwrap();

    let names = tool_names(&mut stdin, &mut stdout, 1);
    assert!(
        !names.contains(&"bb_dispatch".to_string()),
        "bb_dispatch must not be listed unless BB_MCP_ENABLE_DISPATCH is set, got: {names:?}"
    );

    let rejected = call(
        &mut stdin,
        &mut stdout,
        2,
        "bb_dispatch",
        json!({ "repo": dir.path().to_str().unwrap(), "prompt": "hi" }),
    );
    assert!(
        rejected.get("error").is_some(),
        "expected a JSON-RPC error, got: {rejected}"
    );
    assert_eq!(rejected["error"]["code"], -32602);
    assert!(rejected["error"]["message"]
        .as_str()
        .unwrap()
        .contains("BB_MCP_ENABLE_DISPATCH"));

    drop(stdin);
    let status = child.wait().unwrap();
    assert!(status.success(), "mcp serve exited {status}");
}

#[test]
fn bb_dispatch_is_listed_and_rejects_unsafe_inputs_when_enabled() {
    let dir = tempfile::tempdir().unwrap();
    write_dispatch_plane(dir.path());
    let root = dir.path().to_str().unwrap();

    let mut child = spawn_mcp_server(root, true);
    let mut stdin = child.stdin.take().unwrap();
    let mut stdout = child.stdout.take().unwrap();

    let names = tool_names(&mut stdin, &mut stdout, 1);
    assert!(
        names.contains(&"bb_dispatch".to_string()),
        "bb_dispatch should be listed when BB_MCP_ENABLE_DISPATCH=1, got: {names:?}"
    );

    // Missing everything.
    let missing = call(&mut stdin, &mut stdout, 2, "bb_dispatch", json!({}));
    assert_eq!(missing["result"]["isError"], true, "{missing}");
    assert!(missing["result"]["content"][0]["text"]
        .as_str()
        .unwrap()
        .contains("'repo'"));

    // Missing prompt only.
    let missing_prompt = call(
        &mut stdin,
        &mut stdout,
        3,
        "bb_dispatch",
        json!({ "repo": dir.path().to_str().unwrap() }),
    );
    assert_eq!(
        missing_prompt["result"]["isError"], true,
        "{missing_prompt}"
    );
    assert!(missing_prompt["result"]["content"][0]["text"]
        .as_str()
        .unwrap()
        .contains("'prompt'"));

    // Nonexistent repo path.
    let bad_repo = call(
        &mut stdin,
        &mut stdout,
        4,
        "bb_dispatch",
        json!({ "repo": "/nonexistent/path/does-not-exist", "prompt": "hi" }),
    );
    assert_eq!(bad_repo["result"]["isError"], true, "{bad_repo}");

    // Oversized prompt.
    let oversized = "x".repeat((bitterblossom::dispatch::DISPATCH_BRIEF_MAX_BYTES + 1) as usize);
    let too_big = call(
        &mut stdin,
        &mut stdout,
        5,
        "bb_dispatch",
        json!({ "repo": dir.path().to_str().unwrap(), "prompt": oversized }),
    );
    assert_eq!(too_big["result"]["isError"], true, "{too_big}");
    assert!(too_big["result"]["content"][0]["text"]
        .as_str()
        .unwrap()
        .contains("max is"));

    // None of the rejected calls should have created a run row.
    let plane = Plane::load(dir.path()).unwrap();
    let ledger = Ledger::open(&plane.db_path()).unwrap();
    let runs = ledger.list_runs(None, None).unwrap();
    assert!(
        runs.is_empty(),
        "unsafe/missing-input calls must not create run rows, got: {runs:?}"
    );

    drop(stdin);
    let status = child.wait().unwrap();
    assert!(status.success(), "mcp serve exited {status}");
}

#[test]
fn bb_dispatch_creates_the_same_payload_shape_as_the_cli() {
    let dir = tempfile::tempdir().unwrap();
    write_dispatch_plane(dir.path());
    let root = dir.path().to_str().unwrap();
    let repo = dir.path().to_str().unwrap();

    let mut child = spawn_mcp_server(root, true);
    let mut stdin = child.stdin.take().unwrap();
    let mut stdout = child.stdout.take().unwrap();

    let call_result = call(
        &mut stdin,
        &mut stdout,
        1,
        "bb_dispatch",
        json!({
            "repo": repo,
            "prompt": "Trivial parity job: prove the MCP tool builds the CLI's payload.\n",
            "model": "openrouter/parity-demo",
            "label": "mcp-cli-parity"
        }),
    );
    assert_eq!(call_result["result"]["isError"], false, "{call_result}");
    let mcp_result: Value = serde_json::from_str(
        call_result["result"]["content"][0]["text"]
            .as_str()
            .unwrap(),
    )
    .unwrap();
    let mcp_run_id = mcp_result["run_id"].as_str().unwrap().to_string();
    assert_eq!(mcp_result["duplicate"], false);
    assert!(mcp_result["follow_up"]["logs"]
        .as_str()
        .unwrap()
        .contains(&mcp_run_id));

    drop(stdin);
    let status = child.wait().unwrap();
    assert!(status.success(), "mcp serve exited {status}");

    // Same inputs through the real CLI `bb dispatch` path.
    let brief = dir.path().join("brief.md");
    fs::write(
        &brief,
        "Trivial parity job: prove the MCP tool builds the CLI's payload.\n",
    )
    .unwrap();
    let cli_dispatch = Command::new(env!("CARGO_BIN_EXE_bb"))
        .args([
            "--config",
            root,
            "dispatch",
            "--repo",
            repo,
            "--brief",
            brief.to_str().unwrap(),
            "--model",
            "openrouter/parity-demo",
            "--label",
            "mcp-cli-parity-cli",
        ])
        .output()
        .unwrap();
    assert!(
        cli_dispatch.status.success(),
        "stderr:\n{}",
        String::from_utf8_lossy(&cli_dispatch.stderr)
    );
    let cli_run_id = String::from_utf8(cli_dispatch.stdout)
        .unwrap()
        .trim()
        .to_string();

    let plane = Plane::load(dir.path()).unwrap();
    let ledger = Ledger::open(&plane.db_path()).unwrap();
    let mut mcp_payload: Value =
        serde_json::from_str(&ledger.run_payload(&mcp_run_id).unwrap().unwrap()).unwrap();
    let mut cli_payload: Value =
        serde_json::from_str(&ledger.run_payload(&cli_run_id).unwrap().unwrap()).unwrap();
    // Only the deliberately distinct label/branch_slug should differ between
    // the two calls; everything else must match key-for-key.
    mcp_payload["label"] = json!(null);
    mcp_payload["branch_slug"] = json!(null);
    cli_payload["label"] = json!(null);
    cli_payload["branch_slug"] = json!(null);
    assert_eq!(
        mcp_payload, cli_payload,
        "MCP bb_dispatch payload drifted from CLI `bb dispatch` payload"
    );
}

#[test]
fn bb_dispatch_refuses_a_duplicate_unless_forced() {
    let dir = tempfile::tempdir().unwrap();
    write_dispatch_plane(dir.path());
    let root = dir.path().to_str().unwrap();
    let repo = dir.path().to_str().unwrap();

    let mut child = spawn_mcp_server(root, true);
    let mut stdin = child.stdin.take().unwrap();
    let mut stdout = child.stdout.take().unwrap();

    let args = json!({
        "repo": repo,
        "prompt": "Same job, dispatched twice.\n",
        "label": "dupe-check",
        "branch_slug": "dupe-check-branch"
    });

    let first = call(&mut stdin, &mut stdout, 1, "bb_dispatch", args.clone());
    assert_eq!(first["result"]["isError"], false, "{first}");
    let first_result: Value =
        serde_json::from_str(first["result"]["content"][0]["text"].as_str().unwrap()).unwrap();
    assert_eq!(first_result["duplicate"], false);
    let first_run_id = first_result["run_id"].as_str().unwrap().to_string();

    // Second call, same repo/label/branch_slug, no force: refused as a
    // duplicate -- returns the SAME run id, not a new one.
    let second = call(&mut stdin, &mut stdout, 2, "bb_dispatch", args.clone());
    assert_eq!(second["result"]["isError"], false, "{second}");
    let second_result: Value =
        serde_json::from_str(second["result"]["content"][0]["text"].as_str().unwrap()).unwrap();
    assert_eq!(second_result["duplicate"], true, "{second_result}");
    assert_eq!(
        second_result["run_id"].as_str().unwrap(),
        first_run_id,
        "a duplicate dispatch must return the original run id, not mint a new one"
    );

    // Explicit force path: a third call with force:true mints a genuinely
    // new run even though the (repo, label, branch_slug) tuple repeats.
    let mut forced_args = args.clone();
    forced_args["force"] = json!(true);
    let third = call(&mut stdin, &mut stdout, 3, "bb_dispatch", forced_args);
    assert_eq!(third["result"]["isError"], false, "{third}");
    let third_result: Value =
        serde_json::from_str(third["result"]["content"][0]["text"].as_str().unwrap()).unwrap();
    assert_eq!(third_result["duplicate"], false, "{third_result}");
    assert_ne!(
        third_result["run_id"].as_str().unwrap(),
        first_run_id,
        "force:true must mint a fresh run id even for a repeated (repo, label, branch_slug)"
    );

    // Exactly two run rows exist for the "dispatch" task: the original and
    // the forced one. The refused duplicate call never created a third row.
    let plane = Plane::load(dir.path()).unwrap();
    let ledger = Ledger::open(&plane.db_path()).unwrap();
    let runs = ledger.list_runs(Some("dispatch"), None).unwrap();
    assert_eq!(
        runs.len(),
        2,
        "expected exactly 2 runs (original + forced), got: {runs:?}"
    );

    drop(stdin);
    let status = child.wait().unwrap();
    assert!(status.success(), "mcp serve exited {status}");
}

#[test]
fn bb_dispatch_carries_base_ref_through_the_payload() {
    let dir = tempfile::tempdir().unwrap();
    write_dispatch_plane(dir.path());
    let root = dir.path().to_str().unwrap();
    let repo = dir.path().to_str().unwrap();

    let mut child = spawn_mcp_server(root, true);
    let mut stdin = child.stdin.take().unwrap();
    let mut stdout = child.stdout.take().unwrap();

    let result = call(
        &mut stdin,
        &mut stdout,
        1,
        "bb_dispatch",
        json!({
            "repo": repo,
            "prompt": "Base ref pass-through check.\n",
            "label": "base-ref-check",
            "base_ref": "origin/main"
        }),
    );
    assert_eq!(result["result"]["isError"], false, "{result}");
    let parsed: Value =
        serde_json::from_str(result["result"]["content"][0]["text"].as_str().unwrap()).unwrap();
    let run_id = parsed["run_id"].as_str().unwrap();

    drop(stdin);
    let status = child.wait().unwrap();
    assert!(status.success(), "mcp serve exited {status}");

    let plane = Plane::load(dir.path()).unwrap();
    let ledger = Ledger::open(&plane.db_path()).unwrap();
    let payload: Value =
        serde_json::from_str(&ledger.run_payload(run_id).unwrap().unwrap()).unwrap();
    assert_eq!(payload["base_ref"], "origin/main");
    assert_eq!(payload["schema_version"], "bb.dispatch_job.v1");
}
