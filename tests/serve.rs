use std::fs;
use std::io::{Read, Write};
use std::net::{TcpListener, TcpStream};
use std::process::{Child, Command, Output, Stdio};
use std::time::{Duration, Instant};

use bitterblossom::ledger::{IngressRequest, Ledger};
use bitterblossom::spec::Plane;

fn write_plane(root: &std::path::Path) {
    fs::write(
        root.join("plane.toml"),
        "dev = true\n[ingress]\nbind = \"127.0.0.1:0\"\n",
    )
    .unwrap();
}

fn write_dispatch_plane(root: &std::path::Path) {
    write_plane(root);
    fs::create_dir_all(root.join("agents")).unwrap();
    fs::create_dir_all(root.join("tasks/demo")).unwrap();
    let stub = root.join("stub.sh");
    fs::write(&stub, "#!/bin/sh\ncat >/dev/null\necho ok\n").unwrap();
    let mut perms = fs::metadata(&stub).unwrap().permissions();
    #[cfg(unix)]
    {
        use std::os::unix::fs::PermissionsExt;
        perms.set_mode(0o755);
        fs::set_permissions(&stub, perms).unwrap();
    }
    fs::write(
        root.join("agents/stub.toml"),
        format!(
            "harness = \"command\"\nmodel = \"\"\nbin = \"{}\"\n",
            stub.display()
        ),
    )
    .unwrap();
    fs::write(root.join("tasks/demo/card.md"), "demo\n").unwrap();
    fs::write(
        root.join("tasks/demo/task.toml"),
        "agent = \"stub\"\nsubstrate = \"local\"\n[[trigger]]\nkind = \"manual\"\n",
    )
    .unwrap();
}

fn write_trigger_plane(root: &std::path::Path) {
    write_dispatch_plane(root);
    fs::write(
        root.join("tasks/demo/task.toml"),
        "agent = \"stub\"\nsubstrate = \"local\"\n\
         [[trigger]]\nkind = \"manual\"\n\
         [[trigger]]\nkind = \"cron\"\nschedule = \"0 * * * *\"\n\
         [[trigger]]\nkind = \"webhook\"\nroute = \"demo\"\nsecret_env = \"BB_TEST_DEMO\"\ndedupe_key = \"header:X-Demo\"\n\
         [[trigger.filter]]\npointer = \"/repository/full_name\"\nany_of = [\"misty-step/bitterblossom\"]\n",
    )
    .unwrap();
}

fn enqueue(root: &std::path::Path, task: &str) -> String {
    let plane = Plane::load(root).unwrap();
    let mut ledger = Ledger::open(&plane.db_path()).unwrap();
    ledger
        .ingest(IngressRequest {
            task,
            trigger_kind: "manual",
            idempotency_key: None,
            source_event_id: None,
            payload: None,
            parent_run_id: None,
        })
        .unwrap()
        .run_id
}

fn wait_for_run_state(root: &std::path::Path, run_id: &str, state: &str, timeout: Duration) {
    let plane = Plane::load(root).unwrap();
    let ledger = Ledger::open(&plane.db_path()).unwrap();
    let deadline = Instant::now() + timeout;
    loop {
        let run = ledger.run(run_id).unwrap();
        if run.state == state {
            return;
        }
        if Instant::now() >= deadline {
            panic!("run {run_id} stayed {} instead of {state}", run.state);
        }
        std::thread::sleep(Duration::from_millis(50));
    }
}

fn wait_for_exit(mut child: Child, timeout: Duration) -> Output {
    let deadline = Instant::now() + timeout;
    loop {
        if child.try_wait().unwrap().is_some() {
            return child.wait_with_output().unwrap();
        }
        if Instant::now() >= deadline {
            child.kill().unwrap();
            let output = child.wait_with_output().unwrap();
            panic!("bb serve did not exit within {timeout:?}: {output:?}");
        }
        std::thread::sleep(Duration::from_millis(20));
    }
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
        if TcpStream::connect(("127.0.0.1", port)).is_ok() {
            return;
        }
        std::thread::sleep(Duration::from_millis(20));
    }
    panic!("server did not listen on port {port}");
}

fn http_get(port: u16, path: &str, bearer: Option<&str>) -> (u16, String) {
    let mut stream = TcpStream::connect(("127.0.0.1", port)).unwrap();
    let auth = bearer
        .map(|t| format!("Authorization: Bearer {t}\r\n"))
        .unwrap_or_default();
    write!(
        stream,
        "GET {path} HTTP/1.1\r\nHost: 127.0.0.1\r\n{auth}Connection: close\r\n\r\n"
    )
    .unwrap();
    let mut response = String::new();
    stream.read_to_string(&mut response).unwrap();
    let status = response
        .lines()
        .next()
        .unwrap()
        .split_whitespace()
        .nth(1)
        .unwrap()
        .parse()
        .unwrap();
    (status, response)
}

fn response_body(response: &str) -> &str {
    response.split("\r\n\r\n").nth(1).unwrap_or("")
}

struct ChildGuard(Child);

impl Drop for ChildGuard {
    fn drop(&mut self) {
        let _ = self.0.kill();
        let _ = self.0.wait();
    }
}

#[test]
fn dispatch_worker_panic_does_not_strand_task_in_flight() {
    let dir = tempfile::tempdir().unwrap();
    write_dispatch_plane(dir.path());
    let first = enqueue(dir.path(), "demo");
    let second = enqueue(dir.path(), "demo");
    let port = free_loopback_port();

    let child = Command::new(env!("CARGO_BIN_EXE_bb"))
        .args(["--config", dir.path().to_str().unwrap(), "serve"])
        .env("BB_INGRESS_BIND", format!("127.0.0.1:{port}"))
        .env("BB_TEST_PANIC_AFTER_RUN_ID", &first)
        .stdout(Stdio::null())
        .stderr(Stdio::null())
        .spawn()
        .unwrap();
    let _child = ChildGuard(child);
    wait_for_http(port);

    wait_for_run_state(dir.path(), &first, "success", Duration::from_secs(8));
    wait_for_run_state(dir.path(), &second, "success", Duration::from_secs(8));
}

#[test]
fn public_bind_without_api_token_refuses_startup() {
    let dir = tempfile::tempdir().unwrap();
    write_plane(dir.path());

    let child = Command::new(env!("CARGO_BIN_EXE_bb"))
        .args(["--config", dir.path().to_str().unwrap(), "serve"])
        .env("BB_INGRESS_BIND", "0.0.0.0:0")
        .env_remove("BB_API_TOKEN")
        .stdout(Stdio::piped())
        .stderr(Stdio::piped())
        .spawn()
        .unwrap();
    let output = wait_for_exit(child, Duration::from_secs(2));

    assert!(!output.status.success());
    let stderr = String::from_utf8_lossy(&output.stderr);
    assert!(stderr.contains("BB_API_TOKEN must be set"));
}

#[test]
fn public_bind_with_api_token_starts() {
    let dir = tempfile::tempdir().unwrap();
    write_plane(dir.path());

    let mut child = Command::new(env!("CARGO_BIN_EXE_bb"))
        .args(["--config", dir.path().to_str().unwrap(), "serve"])
        .env("BB_INGRESS_BIND", "0.0.0.0:0")
        .env("BB_API_TOKEN", "test-token")
        .stdout(Stdio::null())
        .stderr(Stdio::null())
        .spawn()
        .unwrap();

    std::thread::sleep(Duration::from_millis(300));
    if let Some(status) = child.try_wait().unwrap() {
        panic!("bb serve exited early: {status}");
    }
    child.kill().unwrap();
    child.wait().unwrap();
}

#[test]
fn read_api_requires_bearer_and_rejects_query_token() {
    let dir = tempfile::tempdir().unwrap();
    write_plane(dir.path());
    let port = free_loopback_port();

    let child = Command::new(env!("CARGO_BIN_EXE_bb"))
        .args(["--config", dir.path().to_str().unwrap(), "serve"])
        .env("BB_INGRESS_BIND", format!("127.0.0.1:{port}"))
        .env("BB_API_TOKEN", "test-token")
        .stdout(Stdio::null())
        .stderr(Stdio::null())
        .spawn()
        .unwrap();
    let _child = ChildGuard(child);
    wait_for_http(port);

    assert_eq!(http_get(port, "/api/runs", None).0, 401);
    assert_eq!(http_get(port, "/api/runs", Some("wrong")).0, 401);
    assert_eq!(http_get(port, "/api/runs?token=test-token", None).0, 401);
    assert_eq!(
        http_get(port, "/api/gate?notsubmission=x", Some("test-token")).0,
        400
    );
    let (status, body) = http_get(port, "/api/runs", Some("test-token"));
    assert_eq!(status, 200, "{body}");
    let (status, body) = http_get(port, "/", Some("test-token"));
    assert_eq!(status, 200, "{body}");
}

#[test]
fn tasks_view_reports_trigger_details() {
    let dir = tempfile::tempdir().unwrap();
    write_trigger_plane(dir.path());
    let plane = Plane::load(dir.path()).unwrap();
    let ledger = Ledger::open(&plane.db_path()).unwrap();

    let rows = bitterblossom::serve::tasks_view(&plane, &ledger).unwrap();
    let details = rows[0]["trigger_details"].as_array().unwrap();

    assert_eq!(rows[0]["triggers"], 3);
    assert!(details.iter().any(|t| t["kind"] == "manual"));
    assert!(details
        .iter()
        .any(|t| t["kind"] == "cron" && t["schedule"] == "0 * * * *"));
    assert!(details.iter().any(|t| {
        t["kind"] == "webhook"
            && t["route"] == "demo"
            && t["dedupe_key"] == "header:X-Demo"
            && t["filters"].as_array().unwrap().len() == 1
    }));
}

#[test]
fn operator_html_escapes_trigger_labels_before_inserting_task_rows() {
    let html = include_str!("../src/operator.html");

    assert!(html.contains("map(triggerLabel).join(\"<br>\")"));
    assert!(html.contains("return `cron ${esc(trigger.schedule)}`"));
    assert!(html.contains("return `webhook /hooks/${esc(trigger.route)}`"));
    assert!(html.contains("return esc(trigger.kind);"));
}

#[test]
fn read_api_exposes_dashboard_observability_routes() {
    let dir = tempfile::tempdir().unwrap();
    write_dispatch_plane(dir.path());
    let run_id = enqueue(dir.path(), "demo");
    let plane = Plane::load(dir.path()).unwrap();
    let ledger = Ledger::open(&plane.db_path()).unwrap();
    ledger
        .try_acquire_host_lease("lane-1", "external-run")
        .unwrap();
    drop(ledger);
    drop(plane);
    let port = free_loopback_port();

    let child = Command::new(env!("CARGO_BIN_EXE_bb"))
        .args(["--config", dir.path().to_str().unwrap(), "serve"])
        .env("BB_INGRESS_BIND", format!("127.0.0.1:{port}"))
        .env("BB_API_TOKEN", "test-token")
        .stdout(Stdio::null())
        .stderr(Stdio::null())
        .spawn()
        .unwrap();
    let _child = ChildGuard(child);
    wait_for_http(port);

    for route in [
        "/api/status",
        "/api/tasks",
        "/api/runs",
        "/api/leases",
        "/api/ingress",
        "/api/export",
    ] {
        assert_eq!(http_get(port, route, None).0, 401, "{route}");
        let (status, body) = http_get(port, route, Some("test-token"));
        assert_eq!(status, 200, "{route}: {body}");
    }

    let (_, body) = http_get(port, "/api/leases", Some("test-token"));
    let leases: serde_json::Value = serde_json::from_str(response_body(&body)).unwrap();
    assert_eq!(leases[0]["host"], "lane-1");

    let (_, body) = http_get(port, "/api/ingress", Some("test-token"));
    let ingress: serde_json::Value = serde_json::from_str(response_body(&body)).unwrap();
    assert!(ingress.as_array().unwrap().iter().any(|e| {
        e["task"] == "demo" && e["run_id"] == run_id && e["trigger_kind"] == "manual"
    }));

    let (_, body) = http_get(port, "/api/export", Some("test-token"));
    let first_line = response_body(&body).lines().next().unwrap();
    let exported: serde_json::Value = serde_json::from_str(first_line).unwrap();
    assert_eq!(exported["schema"], "bb.run_telemetry.v1");
}
