//! Fixture/shape tests for the agent-facing JSON contract (backlog 053,
//! narrow first slice, paired with the 077 local-plane baseline). These
//! validate required fields and *types* for the stable agent surfaces —
//! not string presence alone — so a renamed or removed field fails the
//! gate. Additive fields are tolerated. They run against a zero-credential
//! `dev = true` local plane, no secrets, no network.

use std::fs;
use std::process::{Command, Output};
use std::sync::Mutex;

static BB_CMD_LOCK: Mutex<()> = Mutex::new(());

fn write_local_plane(root: &std::path::Path) {
    fs::create_dir_all(root.join("agents")).unwrap();
    fs::create_dir_all(root.join("tasks/hello")).unwrap();
    fs::write(
        root.join("plane.toml"),
        "dev = true\n[budget]\nmax_cost_per_day_usd = 1.0\n",
    )
    .unwrap();
    fs::write(
        root.join("agents/local-command.toml"),
        "version = 1\nharness = \"command\"\nmodel = \"\"\nbin = \"true\"\n",
    )
    .unwrap();
    fs::write(root.join("tasks/hello/card.md"), "card\n").unwrap();
    fs::write(
        root.join("tasks/hello/task.toml"),
        "agent = \"local-command\"\nsubstrate = \"local\"\n[[trigger]]\nkind = \"manual\"\n",
    )
    .unwrap();
}

/// A gate plane with one `verify` verdict member that exits 0 (pass).
fn write_gate_plane(root: &std::path::Path) {
    fs::create_dir_all(root.join("agents")).unwrap();
    fs::create_dir_all(root.join("tasks/verify")).unwrap();
    fs::write(
        root.join("plane.toml"),
        "dev = true\n[gate]\nrequired = [\"verify\"]\nmax_rounds = 3\narbiter = \"arbiter\"\n",
    )
    .unwrap();
    fs::write(
        root.join("agents/verify.toml"),
        "version = 1\nharness = \"command\"\nmodel = \"\"\nbin = \"true\"\n",
    )
    .unwrap();
    fs::write(root.join("tasks/verify/card.md"), "card\n").unwrap();
    fs::write(
        root.join("tasks/verify/task.toml"),
        "agent = \"verify\"\nsubstrate = \"local\"\nverdict = \"verify\"\n[[trigger]]\nkind = \"manual\"\n",
    )
    .unwrap();
}

fn bb(root: &str, args: &[&str]) -> Output {
    let _guard = BB_CMD_LOCK.lock().unwrap();
    Command::new(env!("CARGO_BIN_EXE_bb"))
        .arg("--config")
        .arg(root)
        .args(args)
        .env("BB_RUN_HEARTBEAT_MS", "100")
        .output()
        .unwrap()
}

fn json_ok(root: &str, args: &[&str]) -> serde_json::Value {
    let out = bb(root, args);
    assert!(
        out.status.success(),
        "cmd {:?} failed\nstdout:\n{}\nstderr:\n{}",
        args,
        String::from_utf8_lossy(&out.stdout),
        String::from_utf8_lossy(&out.stderr)
    );
    serde_json::from_slice(&out.stdout).expect("json output")
}

fn as_str<'a>(v: &'a serde_json::Value, k: &str) -> &'a str {
    v[k].as_str()
        .unwrap_or_else(|| panic!("'{k}' not a string: {}", v[k]))
}
fn as_num(v: &serde_json::Value, k: &str) -> f64 {
    v[k].as_number()
        .unwrap_or_else(|| panic!("'{k}' not a number: {}", v[k]))
        .as_f64()
        .unwrap()
}
fn top_arr(v: &serde_json::Value) -> &[serde_json::Value] {
    v.as_array()
        .unwrap_or_else(|| panic!("expected a JSON array, got: {v}"))
}
fn as_arr<'a>(v: &'a serde_json::Value, k: &str) -> &'a [serde_json::Value] {
    v[k].as_array()
        .unwrap_or_else(|| panic!("'{k}' not an array: {}", v[k]))
}

#[test]
fn check_json_shape() {
    let dir = tempfile::tempdir().unwrap();
    write_local_plane(dir.path());
    let root = dir.path().to_str().unwrap();
    let doc = json_ok(root, &["check", "--json"]);
    as_str(&doc, "root");
    assert!(doc["db_path"].is_string());
    assert!(doc["agents"].is_array());
    assert!(doc["tasks"].is_array());
    let task = &as_arr(&doc, "task_details")[0];
    as_str(task, "task");
    as_str(task, "agent");
    as_str(task, "harness");
    as_str(task, "substrate");
    as_num(task, "triggers");
    assert!(task["runs_today"].is_number());
}

#[test]
fn status_json_shape() {
    let dir = tempfile::tempdir().unwrap();
    write_local_plane(dir.path());
    let root = dir.path().to_str().unwrap();
    assert!(bb(root, &["run", "hello", "--json"]).status.success());
    let doc = json_ok(root, &["status", "--json"]);
    let summary = &doc["summary"];
    as_num(summary, "tasks");
    as_num(summary, "open_dlq");
    as_num(summary, "parked_tasks");
    as_num(summary, "max_cost_per_day_usd");
    assert!(summary["cost_today_usd"].is_number());
    let task = &as_arr(&doc, "tasks")[0];
    as_str(task, "task");
    as_str(task, "agent");
    as_str(task, "harness");
    assert!(task["parked"].is_null());
    assert!(task["runs"]["by_state"].is_object());
    assert!(task["safe_next_actions"].is_array());
}

#[test]
fn task_list_json_shape() {
    let dir = tempfile::tempdir().unwrap();
    write_local_plane(dir.path());
    let root = dir.path().to_str().unwrap();
    let rows = json_ok(root, &["task", "list", "--json"]);
    let row = &top_arr(&rows)[0];
    as_str(row, "task");
    as_str(row, "agent");
    as_str(row, "substrate");
    as_num(row, "triggers");
    assert!(row["runs_today"].is_number());
    assert!(row["verdict"].is_null() || row["verdict"].is_string());
    assert!(row["parked"].is_null() || row["parked"].is_string());
    assert!(row["max_runs_per_day"].is_null() || row["max_runs_per_day"].is_number());
}

#[test]
fn runs_list_json_shape() {
    let dir = tempfile::tempdir().unwrap();
    write_local_plane(dir.path());
    let root = dir.path().to_str().unwrap();
    assert!(bb(root, &["run", "hello", "--json"]).status.success());
    let rows = json_ok(root, &["runs", "list", "--json"]);
    let row = &top_arr(&rows)[0];
    as_str(row, "id");
    as_str(row, "task");
    as_str(row, "trigger_kind");
    as_str(row, "state");
    as_str(row, "trace_id");
    as_str(row, "created_at");
    as_str(row, "updated_at");
    assert!(row["agent_name"].is_null() || row["agent_name"].is_string());
    assert!(row["agent_version"].is_null() || row["agent_version"].is_number());
    assert!(row["cost_usd"].is_null() || row["cost_usd"].is_number());
    assert!(row["duration_ms"].is_null() || row["duration_ms"].is_number());
}

#[test]
fn runs_show_json_shape() {
    let dir = tempfile::tempdir().unwrap();
    write_local_plane(dir.path());
    let root = dir.path().to_str().unwrap();
    let run = json_ok(root, &["run", "hello", "--json"]);
    let run_id = as_str(&run["run"], "id");
    let doc = json_ok(root, &["runs", "show", run_id, "--json"]);
    let run = &doc["run"];
    as_str(run, "id");
    as_str(run, "task");
    as_str(run, "state");
    as_str(run, "trigger_kind");
    as_str(run, "trace_id");
    as_str(run, "created_at");
    as_str(run, "updated_at");
    let attempt = &as_arr(&doc, "attempts")[0];
    as_num(attempt, "id");
    as_num(attempt, "n");
    as_str(attempt, "agent_name");
    as_num(attempt, "agent_version");
    as_str(attempt, "harness");
    as_str(attempt, "model");
    as_str(attempt, "phase");
    as_str(attempt, "started_at");
    assert!(attempt["outcome"].is_null() || attempt["outcome"].is_string());
    assert!(attempt["exit_code"].is_null() || attempt["exit_code"].is_number());
    assert!(attempt["ended_at"].is_null() || attempt["ended_at"].is_string());
    assert!(attempt["artifact_dir"].is_null() || attempt["artifact_dir"].is_string());
    let event = &as_arr(&doc, "events")[0];
    as_str(event, "run_id");
    as_str(event, "kind");
    as_str(event, "at");
    assert!(event["data"].is_null() || event["data"].is_string());
}

#[test]
fn dlq_list_json_shape() {
    let dir = tempfile::tempdir().unwrap();
    write_local_plane(dir.path());
    let root = dir.path().to_str().unwrap();
    // A pre_command failure dead-letters before execute.
    fs::write(
        dir.path().join("tasks/hello/task.toml"),
        "agent = \"local-command\"\nsubstrate = \"local\"\npre_command = \"exit 7\"\n[[trigger]]\nkind = \"manual\"\n",
    )
    .unwrap();
    assert!(!bb(root, &["run", "hello", "--json"]).status.success());
    let rows = json_ok(root, &["dlq", "list", "--json"]);
    let row = &top_arr(&rows)[0];
    as_num(row, "id");
    as_str(row, "run_id");
    as_str(row, "task");
    as_str(row, "error");
    as_str(row, "created_at");
    as_str(row, "status");
    assert!(row["payload"].is_null() || row["payload"].is_string());
    assert!(row["replayed_run_id"].is_null() || row["replayed_run_id"].is_string());
    assert!(row["acknowledged_reason"].is_null() || row["acknowledged_reason"].is_string());
    assert!(row["acknowledged_at"].is_null() || row["acknowledged_at"].is_string());
}

#[test]
fn gate_json_shape() {
    let dir = tempfile::tempdir().unwrap();
    write_gate_plane(dir.path());
    let root = dir.path().to_str().unwrap();
    let sub = json_ok(
        root,
        &[
            "submit", "open", "--change", "c1", "--rev", "deadbeef", "--json",
        ],
    );
    let sub_id = as_str(&sub, "id");
    assert!(bb(
        root,
        &[
            "run",
            "verify",
            "--payload",
            &format!("{{\"submission\":\"{sub_id}\"}}"),
            "--json"
        ]
    )
    .status
    .success());
    let report = json_ok(root, &["gate", "--change", "c1", "--json"]);
    as_str(&report, "submission");
    as_str(&report, "change_key");
    as_str(&report, "rev");
    as_num(&report, "round");
    as_num(&report, "max_rounds");
    as_str(&report, "decision");
    let member = &as_arr(&report, "members")[0];
    as_str(member, "kind");
    as_str(member, "status");
    assert!(report["blocking"].is_array());
    assert!(report["advisory"].is_array());
    assert!(report["rejected"].is_array());
}

#[test]
fn run_rejects_invalid_payload_before_ingest() {
    let dir = tempfile::tempdir().unwrap();
    write_local_plane(dir.path());
    let root = dir.path().to_str().unwrap();
    let before = top_arr(&json_ok(root, &["runs", "list", "--json"])).len();
    let bad = bb(root, &["run", "hello", "--payload", "not json"]);
    assert!(!bad.status.success(), "invalid payload must exit non-zero");
    let stderr = String::from_utf8_lossy(&bad.stderr);
    assert!(stderr.contains("--payload is not valid JSON"));
    assert!(
        !stderr.contains("not json"),
        "invalid payload contents must not be echoed to stderr: {stderr}"
    );
    let after = top_arr(&json_ok(root, &["runs", "list", "--json"])).len();
    assert_eq!(before, after, "invalid payload must not create a run row");
}

#[test]
fn run_payload_file_and_exclusivity() {
    let dir = tempfile::tempdir().unwrap();
    write_local_plane(dir.path());
    let root = dir.path().to_str().unwrap();
    let payload_path = dir.path().join("payload.json");
    fs::write(&payload_path, "{\"ok\":true}").unwrap();
    let run = bb(
        root,
        &[
            "run",
            "hello",
            "--payload-file",
            payload_path.to_str().unwrap(),
            "--json",
        ],
    );
    assert!(
        run.status.success(),
        "{}",
        String::from_utf8_lossy(&run.stderr)
    );
    let doc: serde_json::Value = serde_json::from_slice(&run.stdout).unwrap();
    assert_eq!(doc["run"]["state"], "success");

    // Mutually exclusive with --payload: clap rejects with a non-zero exit.
    let invalid_file = dir.path().join("invalid-payload.json");
    fs::write(&invalid_file, "{\"marker\":\"payload-redaction-marker\",}").unwrap();
    let bad_file = bb(
        root,
        &[
            "run",
            "hello",
            "--payload-file",
            invalid_file.to_str().unwrap(),
        ],
    );
    assert!(
        !bad_file.status.success(),
        "invalid --payload-file JSON must exit non-zero"
    );
    let stderr = String::from_utf8_lossy(&bad_file.stderr);
    assert!(stderr.contains("--payload-file is not valid JSON"));
    assert!(
        !stderr.contains("payload-redaction-marker"),
        "invalid payload-file contents must not be echoed to stderr: {stderr}"
    );

    let both = bb(
        root,
        &[
            "run",
            "hello",
            "--payload",
            "{}",
            "--payload-file",
            payload_path.to_str().unwrap(),
        ],
    );
    assert!(
        !both.status.success(),
        "--payload and --payload-file must conflict"
    );
}
