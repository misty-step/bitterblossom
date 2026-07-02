//! Fixture/shape tests for the agent-facing JSON contract (backlog 053,
//! narrow first slice, paired with the 077 local-plane baseline). These
//! validate required fields and *types* for the stable agent surfaces —
//! not string presence alone — so a renamed or removed field fails the
//! gate. Additive fields are tolerated. They run against a zero-credential
//! `dev = true` local plane, no secrets, no network.

use std::fs;
use std::io::{Read, Write};
use std::net::{TcpListener, TcpStream};
use std::process::{Child, Command, Output, Stdio};
use std::sync::Mutex;
use std::time::{Duration, Instant};

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

fn free_loopback_port() -> u16 {
    TcpListener::bind("127.0.0.1:0")
        .unwrap()
        .local_addr()
        .unwrap()
        .port()
}

fn wait_for_http(port: u16) {
    let deadline = Instant::now() + Duration::from_secs(5);
    while Instant::now() < deadline {
        if let Ok(mut stream) = TcpStream::connect(("127.0.0.1", port)) {
            let _ = stream.set_read_timeout(Some(Duration::from_millis(250)));
            let _ = stream
                .write_all(b"GET /health HTTP/1.1\r\nHost: 127.0.0.1\r\nConnection: close\r\n\r\n");
            let mut response = String::new();
            if stream.read_to_string(&mut response).is_ok() && response.starts_with("HTTP/1.1") {
                return;
            }
        }
        std::thread::sleep(Duration::from_millis(20));
    }
    panic!("server did not listen on port {port}");
}

fn http_get_json(port: u16, path: &str) -> serde_json::Value {
    let deadline = Instant::now() + Duration::from_secs(5);
    let request = format!(
        "GET {path} HTTP/1.1\r\nHost: 127.0.0.1\r\nAuthorization: Bearer test-token\r\nConnection: close\r\n\r\n"
    );
    let mut last_error = None;
    while Instant::now() < deadline {
        match TcpStream::connect(("127.0.0.1", port)).and_then(|mut stream| {
            stream.write_all(request.as_bytes())?;
            let mut response = String::new();
            stream.read_to_string(&mut response)?;
            Ok(response)
        }) {
            Ok(response) if response.starts_with("HTTP/1.1 200") => {
                let body = response.split("\r\n\r\n").nth(1).unwrap_or("");
                return serde_json::from_str(body)
                    .unwrap_or_else(|e| panic!("{path} returned invalid json: {e}: {body}"));
            }
            Ok(response) => last_error = Some(response.lines().next().unwrap_or("").to_string()),
            Err(e) => last_error = Some(e.to_string()),
        }
        std::thread::sleep(Duration::from_millis(20));
    }
    panic!(
        "GET {path} on port {port} did not return JSON: {}",
        last_error.unwrap_or_else(|| "timed out".to_string())
    );
}

struct ChildGuard(Child);

impl Drop for ChildGuard {
    fn drop(&mut self) {
        let _ = self.0.kill();
        let _ = self.0.wait();
    }
}

fn start_api(root: &str) -> (u16, ChildGuard) {
    let port = free_loopback_port();
    let child = Command::new(env!("CARGO_BIN_EXE_bb"))
        .args(["--config", root, "serve"])
        .env("BB_INGRESS_BIND", format!("127.0.0.1:{port}"))
        .env("BB_API_TOKEN", "test-token")
        .stdout(Stdio::null())
        .stderr(Stdio::null())
        .spawn()
        .unwrap();
    wait_for_http(port);
    (port, ChildGuard(child))
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

#[derive(Debug)]
struct Requirement {
    surface: String,
    path: String,
    type_name: String,
}

fn read_contract_requirements() -> Vec<Requirement> {
    let doc: serde_json::Value = serde_json::from_str(include_str!(
        "fixtures/contracts/bb.agent_read_surfaces.v1.schema.json"
    ))
    .unwrap();
    assert_eq!(doc["schema_version"], "bb.agent_read_surfaces.v1");
    let mut out = Vec::new();
    for surface in doc["surfaces"].as_array().unwrap() {
        let name = surface["name"].as_str().unwrap().to_string();
        for req in surface["required"].as_array().unwrap() {
            out.push(Requirement {
                surface: name.clone(),
                path: req["path"].as_str().unwrap().to_string(),
                type_name: req["type"].as_str().unwrap().to_string(),
            });
        }
    }
    out
}

fn values_at_path<'a>(value: &'a serde_json::Value, path: &str) -> Vec<&'a serde_json::Value> {
    fn walk<'a>(
        value: &'a serde_json::Value,
        segments: &[&str],
        out: &mut Vec<&'a serde_json::Value>,
    ) {
        if segments.is_empty() {
            out.push(value);
            return;
        }
        let (head, tail) = segments.split_first().unwrap();
        if *head == "*" {
            if let Some(items) = value.as_array() {
                for item in items {
                    walk(item, tail, out);
                }
            }
        } else if let Ok(index) = head.parse::<usize>() {
            if let Some(item) = value.as_array().and_then(|items| items.get(index)) {
                walk(item, tail, out);
            }
        } else if let Some(item) = value.get(*head) {
            walk(item, tail, out);
        }
    }

    let segments: Vec<_> = path.trim_start_matches('/').split('/').collect();
    let mut out = Vec::new();
    walk(value, &segments, &mut out);
    out
}

fn value_matches_type(value: &serde_json::Value, type_name: &str) -> bool {
    match type_name {
        "string" => value.is_string(),
        "number" => value.is_number(),
        "object" => value.is_object(),
        "array" => value.is_array(),
        "boolean" => value.is_boolean(),
        "null" => value.is_null(),
        "string|null" => value.is_string() || value.is_null(),
        "number|null" => value.is_number() || value.is_null(),
        "boolean|null" => value.is_boolean() || value.is_null(),
        other => panic!("unknown contract type {other}"),
    }
}

fn assert_contract_surface(surface: &str, value: &serde_json::Value) {
    let requirements = read_contract_requirements();
    let mut matched = 0;
    for req in requirements.iter().filter(|req| req.surface == surface) {
        matched += 1;
        let values = values_at_path(value, &req.path);
        assert!(
            !values.is_empty(),
            "{surface} missing required path {} in {value}",
            req.path
        );
        for found in values {
            assert!(
                value_matches_type(found, &req.type_name),
                "{surface} path {} expected {}, got {found}",
                req.path,
                req.type_name
            );
        }
    }
    assert!(matched > 0, "no contract requirements for {surface}");
}

#[test]
fn versioned_agent_read_surface_contract_fixture_validates_cli_and_api() {
    let dir = tempfile::tempdir().unwrap();
    write_local_plane(dir.path());
    let root = dir.path().to_str().unwrap();
    let run = json_ok(root, &["run", "hello", "--json"]);
    let run_id = as_str(&run["run"], "id").to_string();
    fs::write(
        dir.path().join("tasks/hello/task.toml"),
        "agent = \"local-command\"\nsubstrate = \"local\"\npre_command = \"exit 7\"\n[[trigger]]\nkind = \"manual\"\n",
    )
    .unwrap();
    assert!(!bb(root, &["run", "hello", "--json"]).status.success());
    let (port, _api) = start_api(root);

    for (surface, doc) in [
        ("task_list", json_ok(root, &["task", "list", "--json"])),
        ("runs_list", json_ok(root, &["runs", "list", "--json"])),
        (
            "runs_show",
            json_ok(root, &["runs", "show", &run_id, "--json"]),
        ),
        ("dlq_list", json_ok(root, &["dlq", "list", "--json"])),
        ("api_tasks", http_get_json(port, "/api/tasks")),
        ("api_runs", http_get_json(port, "/api/runs")),
        (
            "api_runs_show",
            http_get_json(port, &format!("/api/runs/{run_id}")),
        ),
        ("api_dlq", http_get_json(port, "/api/dlq")),
    ] {
        assert_contract_surface(surface, &doc);
    }

    let gate_dir = tempfile::tempdir().unwrap();
    write_gate_plane(gate_dir.path());
    let gate_root = gate_dir.path().to_str().unwrap();
    let sub = json_ok(
        gate_root,
        &[
            "submit", "open", "--change", "c1", "--rev", "deadbeef", "--json",
        ],
    );
    let sub_id = as_str(&sub, "id").to_string();
    assert!(bb(
        gate_root,
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
    let (gate_port, _api) = start_api(gate_root);
    assert_contract_surface(
        "gate",
        &json_ok(gate_root, &["gate", "--change", "c1", "--json"]),
    );
    assert_contract_surface("api_gate", &http_get_json(gate_port, "/api/gate?change=c1"));
    assert_contract_surface(
        "submit_list",
        &json_ok(gate_root, &["submit", "list", "--json"]),
    );
    assert_contract_surface(
        "api_submissions",
        &http_get_json(gate_port, "/api/submissions"),
    );
}

#[test]
fn check_json_shape() {
    let dir = tempfile::tempdir().unwrap();
    write_local_plane(dir.path());
    let root = dir.path().to_str().unwrap();
    let doc = json_ok(root, &["check", "--json"]);
    as_str(&doc, "root");
    assert!(doc["db_path"].is_string());
    assert!(doc["backup"].is_object());
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
    assert!(doc["backup"].is_object());
    as_str(&doc["backup"], "status");
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
fn submit_list_json_shape() {
    let dir = tempfile::tempdir().unwrap();
    write_gate_plane(dir.path());
    let root = dir.path().to_str().unwrap();
    json_ok(
        root,
        &[
            "submit", "open", "--change", "c1", "--rev", "deadbeef", "--json",
        ],
    );
    let c2 = json_ok(
        root,
        &[
            "submit", "open", "--change", "c2", "--rev", "cafebabe", "--json",
        ],
    );
    let c2_id = as_str(&c2, "id");
    assert!(bb(
        root,
        &[
            "run",
            "verify",
            "--idempotency-key",
            &format!("storm:{c2_id}:verify"),
            "--payload",
            &format!("{{\"submission\":\"{c2_id}\"}}"),
            "--json",
        ]
    )
    .status
    .success());
    let report = json_ok(root, &["gate", "--change", "c2", "--json"]);
    assert_eq!(as_str(&report, "decision"), "clear");
    let rows = json_ok(root, &["submit", "list", "--limit", "1", "--json"]);
    assert_eq!(top_arr(&rows).len(), 1, "--limit must constrain output");
    let row = &top_arr(&rows)[0];
    assert_eq!(
        as_str(row, "change_key"),
        "c2",
        "submit list should expose top-level summary fields"
    );
    as_str(row, "id");
    as_str(row, "rev");
    as_num(row, "round");
    as_str(row, "state");
    let sub = &row["submission"];
    assert_eq!(
        as_str(sub, "change_key"),
        "c2",
        "submit list should return newest submissions first"
    );
    as_str(sub, "id");
    as_str(sub, "rev");
    as_num(sub, "round");
    as_str(sub, "state");
    as_str(sub, "created_at");
    as_str(sub, "updated_at");
    let verdict = &as_arr(row, "verdicts")[0];
    as_str(verdict, "kind");
    as_str(verdict, "run_id");
    as_str(verdict, "verdict");
    assert!(verdict["findings"].is_array());
    assert!(row["rejections"].is_array());

    let human = bb(root, &["submit", "list", "--limit", "1"]);
    assert!(human.status.success());
    let stdout = String::from_utf8_lossy(&human.stdout);
    assert!(stdout.contains("change=c2"), "{stdout}");
    assert!(!stdout.contains("change=c1"), "{stdout}");
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
