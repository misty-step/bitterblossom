use std::fs;
use std::io::{Read, Write};
use std::net::TcpStream;
use std::process::{Child, Command, Output, Stdio};
use std::time::{Duration, Instant};

use bitterblossom::ask;
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

fn set_max_body_bytes(root: &std::path::Path, max: usize) {
    fs::write(
        root.join("plane.toml"),
        format!("dev = true\n[ingress]\nbind = \"127.0.0.1:0\"\nmax_body_bytes = {max}\n"),
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

/// Backlog bitterblossom-926: the old `free_loopback_port` bound an ephemeral
/// port, immediately read it back, then dropped the listener before `bb
/// serve` bound the same port number -- a real TOCTOU window under the test
/// suite's default concurrency (every test here spawns its own `bb serve`).
/// The fix removes the window entirely: `bb serve` binds `127.0.0.1:0` itself
/// (already the tests' own `plane.toml` default) and reports the port it
/// actually got via `BB_INGRESS_REPORT_PORT_FILE`, so there is never a gap
/// between "a port is free" and "this process holds it".
fn wait_for_port_file(path: &std::path::Path) -> u16 {
    let deadline = Instant::now() + Duration::from_secs(5);
    loop {
        if let Ok(text) = fs::read_to_string(path) {
            if let Ok(port) = text.trim().parse::<u16>() {
                return port;
            }
        }
        if Instant::now() >= deadline {
            panic!("bb serve never reported its bound port at {path:?} within 5s");
        }
        std::thread::sleep(Duration::from_millis(20));
    }
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

fn http_get(port: u16, path: &str, bearer: Option<&str>) -> (u16, String) {
    http_request(port, "GET", path, bearer, None)
}

fn http_request(
    port: u16,
    method: &str,
    path: &str,
    bearer: Option<&str>,
    body: Option<&str>,
) -> (u16, String) {
    http_request_with_headers(port, method, path, bearer, &[], body)
}

fn http_request_with_headers(
    port: u16,
    method: &str,
    path: &str,
    bearer: Option<&str>,
    extra_headers: &[(&str, &str)],
    body: Option<&str>,
) -> (u16, String) {
    let deadline = Instant::now() + Duration::from_secs(5);
    let auth = bearer
        .map(|t| format!("Authorization: Bearer {t}\r\n"))
        .unwrap_or_default();
    let extra_headers = extra_headers
        .iter()
        .map(|(name, value)| format!("{name}: {value}\r\n"))
        .collect::<String>();
    let body = body.unwrap_or("");
    let content_headers = if body.is_empty() {
        String::new()
    } else {
        format!(
            "Content-Type: application/json\r\nContent-Length: {}\r\n",
            body.len()
        )
    };
    let request = format!(
        "{method} {path} HTTP/1.1\r\nHost: 127.0.0.1\r\n{auth}{extra_headers}{content_headers}Connection: close\r\n\r\n{body}"
    );
    let mut last_error = None;
    while Instant::now() < deadline {
        match TcpStream::connect(("127.0.0.1", port)).and_then(|mut stream| {
            stream.write_all(request.as_bytes())?;
            let mut response = String::new();
            stream.read_to_string(&mut response)?;
            Ok(response)
        }) {
            Ok(response) if response.starts_with("HTTP/1.1") => {
                let status = response
                    .lines()
                    .next()
                    .unwrap()
                    .split_whitespace()
                    .nth(1)
                    .unwrap()
                    .parse()
                    .unwrap();
                return (status, response);
            }
            Ok(response) => last_error = Some(format!("malformed response: {response:?}")),
            Err(e) => last_error = Some(e.to_string()),
        }
        std::thread::sleep(Duration::from_millis(20));
    }
    panic!(
        "{method} {path} on port {port} did not return HTTP response: {}",
        last_error.unwrap_or_else(|| "timed out".to_string())
    );
}

fn response_body(response: &str) -> &str {
    response.split("\r\n\r\n").nth(1).unwrap_or("")
}

fn http_oversized_stream_probe(
    port: u16,
    method: &str,
    path: &str,
    bearer: Option<&str>,
    written_body_bytes: usize,
) -> (u16, String) {
    let auth = bearer
        .map(|t| format!("Authorization: Bearer {t}\r\n"))
        .unwrap_or_default();
    let request = format!(
        "{method} {path} HTTP/1.1\r\nHost: 127.0.0.1\r\n{auth}Content-Type: application/json\r\nContent-Length: 1000000\r\nConnection: close\r\n\r\n"
    );
    let mut stream = TcpStream::connect(("127.0.0.1", port)).unwrap();
    stream
        .set_read_timeout(Some(Duration::from_secs(2)))
        .unwrap();
    stream.write_all(request.as_bytes()).unwrap();
    stream.write_all(&vec![b'x'; written_body_bytes]).unwrap();

    let mut buf = [0_u8; 4096];
    let n = stream
        .read(&mut buf)
        .expect("server should answer before the advertised body is fully sent");
    let response = String::from_utf8_lossy(&buf[..n]).to_string();
    let status = response
        .lines()
        .next()
        .unwrap_or("")
        .split_whitespace()
        .nth(1)
        .unwrap_or("0")
        .parse()
        .unwrap();
    (status, response)
}

#[test]
fn malformed_webhook_signature_returns_401_and_keeps_server_alive() {
    let dir = tempfile::tempdir().unwrap();
    write_trigger_plane(dir.path());
    let port_file = dir.path().join("bb-serve-port");

    let child = Command::new(env!("CARGO_BIN_EXE_bb"))
        .args(["--config", dir.path().to_str().unwrap(), "serve"])
        .env("BB_INGRESS_REPORT_PORT_FILE", &port_file)
        .env("BB_TEST_DEMO", "hunter2")
        .stdout(Stdio::null())
        .stderr(Stdio::null())
        .spawn()
        .unwrap();
    let _child = ChildGuard(child);
    let port = wait_for_port_file(&port_file);
    wait_for_http(port);

    let (status, response) = http_request_with_headers(
        port,
        "POST",
        "/hooks/demo",
        None,
        &[
            ("X-Hub-Signature-256", "sha256=not-hex"),
            ("X-Demo", "panic-repro"),
        ],
        Some(r#"{"repository":{"full_name":"misty-step/bitterblossom"}}"#),
    );
    assert_eq!(status, 401, "{response}");

    let (status, response) = http_get(port, "/health", None);
    assert_eq!(status, 200, "{response}");
}

#[test]
fn webhook_body_cap_answers_before_buffering_full_stream() {
    let dir = tempfile::tempdir().unwrap();
    write_trigger_plane(dir.path());
    set_max_body_bytes(dir.path(), 8);
    let port_file = dir.path().join("bb-serve-port");

    let child = Command::new(env!("CARGO_BIN_EXE_bb"))
        .args(["--config", dir.path().to_str().unwrap(), "serve"])
        .env("BB_INGRESS_REPORT_PORT_FILE", &port_file)
        .env("BB_TEST_DEMO", "hunter2")
        .stdout(Stdio::null())
        .stderr(Stdio::null())
        .spawn()
        .unwrap();
    let _child = ChildGuard(child);
    let port = wait_for_port_file(&port_file);
    wait_for_http(port);

    let (status, response) = http_oversized_stream_probe(port, "POST", "/hooks/demo", None, 9);
    assert_eq!(status, 413, "{response}");
}

#[test]
fn external_runs_body_cap_answers_before_buffering_full_stream() {
    let dir = tempfile::tempdir().unwrap();
    write_plane(dir.path());
    set_max_body_bytes(dir.path(), 8);
    let port_file = dir.path().join("bb-serve-port");

    let child = Command::new(env!("CARGO_BIN_EXE_bb"))
        .args(["--config", dir.path().to_str().unwrap(), "serve"])
        .env("BB_INGRESS_REPORT_PORT_FILE", &port_file)
        .env("BB_API_TOKEN", "test-token")
        .stdout(Stdio::null())
        .stderr(Stdio::null())
        .spawn()
        .unwrap();
    let _child = ChildGuard(child);
    let port = wait_for_port_file(&port_file);
    wait_for_http(port);

    let (status, response) =
        http_oversized_stream_probe(port, "POST", "/api/external-runs", Some("test-token"), 9);
    assert_eq!(status, 413, "{response}");
}

// bitterblossom-930: HITL ask/answer HTTP surface. These tests manufacture a
// "running" run directly through the ledger (bypassing a real dispatched
// attempt, already covered by tests/e2e_local.rs's parked-run tests) so they
// can exercise the raise/poll/answer routes' own auth and state machine in
// isolation.
fn running_run_with_ask_token(root: &std::path::Path, task: &str, token: &str) -> String {
    let plane = Plane::load(root).unwrap();
    let mut ledger = Ledger::open(&plane.db_path()).unwrap();
    let run_id = ledger
        .ingest(IngressRequest {
            task,
            trigger_kind: "manual",
            idempotency_key: None,
            source_event_id: None,
            payload: None,
            parent_run_id: None,
        })
        .unwrap()
        .run_id;
    ledger.transition(&run_id, "running", None).unwrap();
    ledger.set_run_ask_token(&run_id, token).unwrap();
    run_id
}

fn write_run_json(
    dir: &std::path::Path,
    run_id: &str,
    task: &str,
    ask_token: &str,
) -> std::path::PathBuf {
    let path = dir.join("RUN.json");
    fs::write(
        &path,
        serde_json::json!({"run_id": run_id, "task": task, "ask_token": ask_token}).to_string(),
    )
    .unwrap();
    path
}

/// The CLI end of the primitive: a dispatched attempt shells out to `bb ask
/// raise` from its own workspace, exactly like a real card would. This
/// exercises `src/ask.rs` for real (curl, JSON parsing, the poll loop) rather
/// than only the HTTP routes it talks to.
#[test]
fn bb_ask_raise_cli_answers_within_window_and_parks_past_it() {
    let dir = tempfile::tempdir().unwrap();
    write_dispatch_plane(dir.path());
    let port_file = dir.path().join("bb-serve-port");
    let child = Command::new(env!("CARGO_BIN_EXE_bb"))
        .args(["--config", dir.path().to_str().unwrap(), "serve"])
        .env("BB_INGRESS_REPORT_PORT_FILE", &port_file)
        .env("BB_API_TOKEN", "test-token")
        .stdout(Stdio::null())
        .stderr(Stdio::null())
        .spawn()
        .unwrap();
    let _child = ChildGuard(child);
    let port = wait_for_port_file(&port_file);
    wait_for_http(port);
    let base_url = format!("http://127.0.0.1:{port}");

    // Fast path: `bb ask raise` blocks polling until answered, then prints
    // the answer to stdout and exits 0.
    let run_id = running_run_with_ask_token(dir.path(), "demo", "cli-ask-secret-1");
    let run_json = write_run_json(dir.path(), &run_id, "demo", "cli-ask-secret-1");
    let raiser = Command::new(env!("CARGO_BIN_EXE_bb"))
        .args([
            "ask",
            "raise",
            "may I proceed?",
            "--semantics",
            "blocking",
            "--window-seconds",
            "600",
            "--api-base-url",
            &base_url,
            "--run-json",
            run_json.to_str().unwrap(),
        ])
        .stdout(Stdio::piped())
        .stderr(Stdio::piped())
        .spawn()
        .unwrap();
    // Give the raise a moment to land before answering it via the CLI's own
    // operator-facing command (exercises both `ask raise` and `ask answer`).
    let ask_id = wait_for_ask(dir.path(), &run_id);
    let answer_out = Command::new(env!("CARGO_BIN_EXE_bb"))
        .args([
            "ask",
            "answer",
            &ask_id,
            "yes, proceed",
            "--api-base-url",
            &base_url,
        ])
        .env("BB_API_TOKEN", "test-token")
        .output()
        .unwrap();
    assert!(
        answer_out.status.success(),
        "{}",
        String::from_utf8_lossy(&answer_out.stderr)
    );
    let raise_out = wait_for_exit(raiser, Duration::from_secs(10));
    assert!(raise_out.status.success(), "{raise_out:?}");
    assert_eq!(
        String::from_utf8_lossy(&raise_out.stdout).trim(),
        "yes, proceed"
    );

    // Parked path: window_seconds = 0 means any elapsed time parks it, so
    // `bb ask raise` must exit the documented parked code without an answer.
    let run_id_2 = running_run_with_ask_token(dir.path(), "demo", "cli-ask-secret-2");
    let run_json_2 = write_run_json(dir.path(), &run_id_2, "demo", "cli-ask-secret-2");
    let raiser_2 = Command::new(env!("CARGO_BIN_EXE_bb"))
        .args([
            "ask",
            "raise",
            "deploy?",
            "--kind",
            "approval",
            "--semantics",
            "blocking",
            "--window-seconds",
            "0",
            "--api-base-url",
            &base_url,
            "--run-json",
            run_json_2.to_str().unwrap(),
        ])
        .stdout(Stdio::piped())
        .stderr(Stdio::piped())
        .spawn()
        .unwrap();
    let parked_out = wait_for_exit(raiser_2, Duration::from_secs(10));
    assert_eq!(parked_out.status.code(), Some(ask::PARKED_EXIT_CODE));
}

fn wait_for_ask(root: &std::path::Path, run_id: &str) -> String {
    let deadline = Instant::now() + Duration::from_secs(5);
    loop {
        let plane = Plane::load(root).unwrap();
        let ledger = Ledger::open(&plane.db_path()).unwrap();
        if let Some(ask) = ledger.asks_for_run(run_id).unwrap().into_iter().next() {
            return ask.id;
        }
        if Instant::now() >= deadline {
            panic!("no ask raised for run {run_id} within 5s");
        }
        std::thread::sleep(Duration::from_millis(20));
    }
}

#[test]
fn ask_raise_and_answer_fast_path_without_parking() {
    let dir = tempfile::tempdir().unwrap();
    write_dispatch_plane(dir.path());
    let port_file = dir.path().join("bb-serve-port");
    let child = Command::new(env!("CARGO_BIN_EXE_bb"))
        .args(["--config", dir.path().to_str().unwrap(), "serve"])
        .env("BB_INGRESS_REPORT_PORT_FILE", &port_file)
        .env("BB_API_TOKEN", "test-token")
        .stdout(Stdio::null())
        .stderr(Stdio::null())
        .spawn()
        .unwrap();
    let _child = ChildGuard(child);
    let port = wait_for_port_file(&port_file);
    wait_for_http(port);

    let run_id = running_run_with_ask_token(dir.path(), "demo", "ask-secret-1");

    // Wrong token is refused before an ask row is even considered.
    let (status, _) = http_request(
        port,
        "POST",
        "/api/asks",
        Some("wrong-token"),
        Some(&format!(
            r#"{{"run_id":"{run_id}","task":"demo","kind":"question","question":"proceed?","blocking":true,"window_seconds":600}}"#
        )),
    );
    assert_eq!(status, 401);

    // An unknown kind is refused before touching the ledger.
    let (status, response) = http_request(
        port,
        "POST",
        "/api/asks",
        Some("ask-secret-1"),
        Some(&format!(
            r#"{{"run_id":"{run_id}","task":"demo","kind":"bogus","question":"proceed?","blocking":true,"window_seconds":600}}"#
        )),
    );
    assert_eq!(status, 400, "{response}");

    // Polling an ask that does not exist is a 404, not a stale row.
    let (status, _) = http_get(port, "/api/asks/ask-does-not-exist", Some("ask-secret-1"));
    assert_eq!(status, 404);

    // A POST under /api/asks/<id>/ with no recognized suffix is a 404, not a
    // silent fallthrough to the answer handler.
    let (status, _) = http_request(
        port,
        "POST",
        "/api/asks/some-id/not-answer",
        Some("test-token"),
        Some("{}"),
    );
    assert_eq!(status, 404);

    let (status, response) = http_request(
        port,
        "POST",
        "/api/asks",
        Some("ask-secret-1"),
        Some(&format!(
            r#"{{"run_id":"{run_id}","task":"demo","kind":"question","question":"proceed?","blocking":true,"window_seconds":600}}"#
        )),
    );
    assert_eq!(status, 201, "{response}");
    let raised: serde_json::Value = serde_json::from_str(response_body(&response)).unwrap();
    let ask_id = raised["id"].as_str().unwrap().to_string();
    assert_eq!(raised["state"], "open");

    // Poll before an answer exists: still open.
    let (status, response) = http_get(port, &format!("/api/asks/{ask_id}"), Some("ask-secret-1"));
    assert_eq!(status, 200, "{response}");
    let polled: serde_json::Value = serde_json::from_str(response_body(&response)).unwrap();
    assert_eq!(polled["state"], "open");

    // Operator answers -- still within the window, so no resume run.
    let (status, response) = http_request(
        port,
        "POST",
        &format!("/api/asks/{ask_id}/answer"),
        Some("test-token"),
        Some(r#"{"answer":"go ahead","answered_by":"operator"}"#),
    );
    assert_eq!(status, 200, "{response}");
    let answered: serde_json::Value = serde_json::from_str(response_body(&response)).unwrap();
    assert_eq!(answered["ask"]["state"], "answered");
    assert_eq!(answered["ask"]["answer"], "go ahead");
    assert!(answered["resumed_run_id"].is_null());

    // A second answer is refused -- already answered.
    let (status, _) = http_request(
        port,
        "POST",
        &format!("/api/asks/{ask_id}/answer"),
        Some("test-token"),
        Some(r#"{"answer":"different","answered_by":"operator"}"#),
    );
    assert_eq!(status, 409);

    // The raising attempt's next poll sees the answer.
    let (status, response) = http_get(port, &format!("/api/asks/{ask_id}"), Some("ask-secret-1"));
    assert_eq!(status, 200, "{response}");
    let polled: serde_json::Value = serde_json::from_str(response_body(&response)).unwrap();
    assert_eq!(polled["state"], "answered");
    assert_eq!(polled["answer"], "go ahead");
}

#[test]
fn ask_parks_after_window_and_answering_creates_a_lineage_linked_resume_run() {
    let dir = tempfile::tempdir().unwrap();
    write_dispatch_plane(dir.path());
    let port_file = dir.path().join("bb-serve-port");
    let child = Command::new(env!("CARGO_BIN_EXE_bb"))
        .args(["--config", dir.path().to_str().unwrap(), "serve"])
        .env("BB_INGRESS_REPORT_PORT_FILE", &port_file)
        .env("BB_API_TOKEN", "test-token")
        .stdout(Stdio::null())
        .stderr(Stdio::null())
        .spawn()
        .unwrap();
    let _child = ChildGuard(child);
    let port = wait_for_port_file(&port_file);
    wait_for_http(port);

    let run_id = running_run_with_ask_token(dir.path(), "demo", "ask-secret-2");

    let (status, response) = http_request(
        port,
        "POST",
        "/api/asks",
        Some("ask-secret-2"),
        Some(&format!(
            r#"{{"run_id":"{run_id}","task":"demo","kind":"approval","question":"deploy?","blocking":true,"window_seconds":0}}"#
        )),
    );
    assert_eq!(status, 201, "{response}");
    let raised: serde_json::Value = serde_json::from_str(response_body(&response)).unwrap();
    let ask_id = raised["id"].as_str().unwrap().to_string();

    // Poll immediately: window_seconds = 0 means any elapsed time parks it.
    let (status, response) = http_get(port, &format!("/api/asks/{ask_id}"), Some("ask-secret-2"));
    assert_eq!(status, 200, "{response}");
    let polled: serde_json::Value = serde_json::from_str(response_body(&response)).unwrap();
    assert_eq!(polled["state"], "parked");

    // Mirror what dispatch.rs does on a real park: finalize the run itself.
    {
        let plane = Plane::load(dir.path()).unwrap();
        let ledger = Ledger::open(&plane.db_path()).unwrap();
        ledger.transition(&run_id, "parked_on_ask", None).unwrap();
    }

    let (status, response) = http_request(
        port,
        "POST",
        &format!("/api/asks/{ask_id}/answer"),
        Some("test-token"),
        Some(r#"{"answer":"approve","answered_by":"operator"}"#),
    );
    assert_eq!(status, 200, "{response}");
    let answered: serde_json::Value = serde_json::from_str(response_body(&response)).unwrap();
    let resumed_run_id = answered["resumed_run_id"]
        .as_str()
        .expect("a parked ask's answer creates a resume run")
        .to_string();
    assert_ne!(resumed_run_id, run_id);

    let plane = Plane::load(dir.path()).unwrap();
    let ledger = Ledger::open(&plane.db_path()).unwrap();
    let resumed = ledger.run(&resumed_run_id).unwrap();
    assert_eq!(resumed.trigger_kind, "resume");
    assert_eq!(resumed.parent_run_id.as_deref(), Some(run_id.as_str()));
    assert_eq!(
        resumed.idempotency_key.as_deref(),
        Some(format!("resume:{ask_id}").as_str())
    );
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
    let port_file = dir.path().join("bb-serve-port");

    let child = Command::new(env!("CARGO_BIN_EXE_bb"))
        .args(["--config", dir.path().to_str().unwrap(), "serve"])
        .env("BB_INGRESS_REPORT_PORT_FILE", &port_file)
        .env("BB_TEST_PANIC_AFTER_RUN_ID", &first)
        .stdout(Stdio::null())
        .stderr(Stdio::null())
        .spawn()
        .unwrap();
    let _child = ChildGuard(child);
    let port = wait_for_port_file(&port_file);
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
    let port_file = dir.path().join("bb-serve-port");

    let child = Command::new(env!("CARGO_BIN_EXE_bb"))
        .args(["--config", dir.path().to_str().unwrap(), "serve"])
        .env("BB_INGRESS_REPORT_PORT_FILE", &port_file)
        .env("BB_API_TOKEN", "test-token")
        .stdout(Stdio::null())
        .stderr(Stdio::null())
        .spawn()
        .unwrap();
    let _child = ChildGuard(child);
    let port = wait_for_port_file(&port_file);
    wait_for_http(port);

    assert_eq!(http_get(port, "/api/runs", None).0, 401);
    assert_eq!(http_get(port, "/api/runs", Some("wrong")).0, 401);
    assert_eq!(http_get(port, "/api/runs?token=test-token", None).0, 401);
    let (status, body) = http_get(port, "/", None);
    assert_eq!(status, 200, "{body}");
    assert!(body.contains("id=\"authPanel\""));
    assert!(body.contains("localStorage"));

    // Every dashboard data source shares the same `read_authorized` gate --
    // prove the query-token bypass is closed on more than just /api/runs, and
    // that /api/submissions and /api/gate (which take request-specific query
    // params) are gated before those params are even parsed.
    for route in [
        "/api/dlq",
        "/api/submissions",
        "/api/notify",
        "/api/gate?submission=missing",
    ] {
        assert_eq!(http_get(port, route, None).0, 401, "{route}");
        assert_eq!(
            http_get(
                port,
                &format!(
                    "{route}{}token=test-token",
                    if route.contains('?') { "&" } else { "?" }
                ),
                None
            )
            .0,
            401,
            "{route} must not authenticate via a query-string token"
        );
    }

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
fn operator_html_stores_token_locally_and_reprompts_on_unauthorized_api() {
    let html = include_str!("../src/operator.html");

    assert!(html.contains("localStorage.getItem(\"bb-api-token\")"));
    assert!(html.contains("localStorage.setItem(\"bb-api-token\""));
    assert!(html.contains("showAuth"));
    assert!(html.contains("Token rejected"));
    assert!(!html.contains("sessionStorage"));
}

#[test]
fn operator_html_unlocks_the_shell_on_a_non_auth_fetch_error() {
    // bitterblossom-119: a stored-but-valid token combined with a non-401
    // fetch failure (network down, 5xx, oversized response) used to leave
    // the auth overlay locked over the shell forever -- the error banner
    // was written and marked visible, but nobody could ever see it, since
    // only showDashboard() (called on success) or the auth-required branch
    // dismissed the overlay. Pins that the generic-error branch now also
    // calls showDashboard() so the banner is reachable, and resets the
    // plane-sentence placeholder instead of leaving a stale "loading plane".
    let html = include_str!("../src/operator.html");

    let catch_block = html
        .split("} catch (error) {")
        .nth(1)
        .expect("load() has a catch block");
    let generic_error_branch = catch_block
        .split("if (error.authRequired) {")
        .nth(1)
        .and_then(|rest| rest.split_once("    }\n"))
        .map(|(_, after)| after)
        .expect("catch block has an auth-required branch followed by the generic path");

    assert!(
        generic_error_branch.contains("showDashboard();"),
        "the non-auth error path must unlock the shell so the error banner is visible: {generic_error_branch}"
    );
    assert!(generic_error_branch.contains("$(\"error\").classList.add(\"is-visible\")"));
    assert!(generic_error_branch.contains("planeSentence"));
}

#[test]
fn operator_html_renders_submissions_and_gates_from_existing_read_apis() {
    let html = include_str!("../src/operator.html");

    // bitterblossom-118: submissions/gates were entirely absent from the
    // dashboard. Pin that the view exists, is wired to the same /api/*
    // helpers serve.rs already exposes (no new backend), and links out to
    // /api/gate for full quorum detail rather than duplicating gate logic
    // in JS.
    assert!(html.contains(r#"data-view-button="submissions""#));
    assert!(html.contains(r#"data-view="submissions""#));
    assert!(html.contains(r#"fetchJson("/api/submissions?limit=50")"#));
    assert!(html.contains("function renderSubmissions()"));
    assert!(html.contains("function verdictSummary("));
    assert!(html.contains("/api/gate?submission="));
}

#[test]
fn operator_html_renders_notify_outbox_as_a_table_not_only_a_count() {
    let html = include_str!("../src/operator.html");

    // Before this card, /api/notify was only ever surfaced as an aggregate
    // pending/failed count in the proof strip -- pin that the raw outbox
    // rows are now fetched and rendered.
    assert!(html.contains(r#"fetchJson("/api/notify?limit=50")"#));
    assert!(html.contains(r#"id="notifyRows""#));
    assert!(html.contains("function notifyStatusTone("));
}

#[test]
fn operator_html_surfaces_per_run_stale_fresh_classification() {
    let html = include_str!("../src/operator.html");

    // progress::classify's per-run "stale_executing"/"on_track" verdict was
    // only ever aggregated into a freshness_contracts count; pin that the
    // Runs table now carries it per row, keyed off the same
    // status.tasks[].progress.running data serve.rs already returns.
    assert!(html.contains("function runFreshnessMap()"));
    assert!(html.contains("task.progress?.running"));
    assert!(html.contains("function freshnessCell("));
    assert!(html.contains("<th>freshness</th>"));
}

#[test]
fn operator_html_surfaces_provider_key_status_per_task() {
    let html = include_str!("../src/operator.html");

    assert!(html.contains("function providerKeyCell("));
    assert!(html.contains("task.provider_key"));
}

#[test]
fn operator_html_distinguishes_external_runs_from_native_rows() {
    let html = include_str!("../src/operator.html");

    assert!(html.contains("status.external_runs?.recent"));
    assert!(html.contains("sourceLabel(run)"));
    assert!(html.contains("run.source === \"external\""));
    assert!(html.contains("external runs"));
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
    let port_file = dir.path().join("bb-serve-port");

    let child = Command::new(env!("CARGO_BIN_EXE_bb"))
        .args(["--config", dir.path().to_str().unwrap(), "serve"])
        .env("BB_INGRESS_REPORT_PORT_FILE", &port_file)
        .env("BB_API_TOKEN", "test-token")
        .stdout(Stdio::null())
        .stderr(Stdio::null())
        .spawn()
        .unwrap();
    let _child = ChildGuard(child);
    let port = wait_for_port_file(&port_file);
    wait_for_http(port);

    for route in [
        "/api/status",
        "/api/tasks",
        "/api/runs",
        "/api/notify",
        "/api/leases",
        "/api/ingress",
        "/api/export",
        "/api/dlq",
        "/api/submissions",
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

#[test]
fn external_runs_api_registers_patches_and_status_surfaces_source() {
    let dir = tempfile::tempdir().unwrap();
    write_plane(dir.path());
    let port_file = dir.path().join("bb-serve-port");

    let child = Command::new(env!("CARGO_BIN_EXE_bb"))
        .args(["--config", dir.path().to_str().unwrap(), "serve"])
        .env("BB_INGRESS_REPORT_PORT_FILE", &port_file)
        .env("BB_API_TOKEN", "test-token")
        .stdout(Stdio::null())
        .stderr(Stdio::null())
        .spawn()
        .unwrap();
    let _child = ChildGuard(child);
    let port = wait_for_port_file(&port_file);
    wait_for_http(port);

    let body = serde_json::json!({
        "agent": "codex-bb-909",
        "role": "implementer",
        "repo": "misty-step/bitterblossom",
        "brief_hash": "sha256:test",
        "plane": "local",
        "status_url": "https://example.test/status",
        "receipt_path": "/tmp/bb-909.md",
        "started_at": "2026-07-04T12:00:00Z"
    })
    .to_string();
    assert_eq!(
        http_request(port, "POST", "/api/external-runs", None, Some(&body)).0,
        401
    );
    let (status, response) = http_request(
        port,
        "POST",
        "/api/external-runs",
        Some("test-token"),
        Some(&body),
    );
    assert_eq!(status, 201, "{response}");
    let created: serde_json::Value = serde_json::from_str(response_body(&response)).unwrap();
    let id = created["id"].as_str().unwrap();
    assert_eq!(created["source"], "external");
    assert_eq!(created["status"], "running");

    let patch = serde_json::json!({
        "status": "done",
        "completed_at": "2026-07-04T12:05:00Z"
    })
    .to_string();
    let (status, response) = http_request(
        port,
        "PATCH",
        &format!("/api/external-runs/{id}"),
        Some("test-token"),
        Some(&patch),
    );
    assert_eq!(status, 200, "{response}");
    let patched: serde_json::Value = serde_json::from_str(response_body(&response)).unwrap();
    assert_eq!(patched["status"], "done");
    assert_eq!(patched["completed_at"], "2026-07-04T12:05:00Z");

    let (status, response) = http_get(port, "/api/status", Some("test-token"));
    assert_eq!(status, 200, "{response}");
    let status_doc: serde_json::Value = serde_json::from_str(response_body(&response)).unwrap();
    let rows = status_doc["external_runs"]["recent"].as_array().unwrap();
    let row = rows.iter().find(|row| row["id"] == id).unwrap();
    assert_eq!(row["source"], "external");
    assert_eq!(row["agent"], "codex-bb-909");
    assert_eq!(row["status"], "done");
    assert_eq!(status_doc["summary"]["external_runs"], 1);
    assert_eq!(status_doc["summary"]["external_running"], 0);
}

#[test]
fn submissions_read_api_exposes_top_level_identity_summary() {
    let dir = tempfile::tempdir().unwrap();
    write_plane(dir.path());
    let plane = Plane::load(dir.path()).unwrap();
    let mut ledger = Ledger::open(&plane.db_path()).unwrap();
    let sub = ledger
        .open_submission(
            "refs/pull/910",
            "fee9bf7509da994e4a7302685f781d2f3462c1e8",
            None,
        )
        .unwrap();
    drop(ledger);
    drop(plane);
    let port_file = dir.path().join("bb-serve-port");

    let child = Command::new(env!("CARGO_BIN_EXE_bb"))
        .args(["--config", dir.path().to_str().unwrap(), "serve"])
        .env("BB_INGRESS_REPORT_PORT_FILE", &port_file)
        .env("BB_API_TOKEN", "test-token")
        .stdout(Stdio::null())
        .stderr(Stdio::null())
        .spawn()
        .unwrap();
    let _child = ChildGuard(child);
    let port = wait_for_port_file(&port_file);
    wait_for_http(port);

    let (status, body) = http_get(port, "/api/submissions?limit=1", Some("test-token"));
    assert_eq!(status, 200, "{body}");
    let rows: serde_json::Value = serde_json::from_str(response_body(&body)).unwrap();
    let row = &rows.as_array().unwrap()[0];

    assert_eq!(row["id"], sub.id);
    assert_eq!(row["change_key"], "refs/pull/910");
    assert_eq!(row["rev"], "fee9bf7509da994e4a7302685f781d2f3462c1e8");
    assert_eq!(row["round"], 1);
    assert_eq!(row["state"], "open");
    assert_eq!(row["submission"]["id"], sub.id);
    assert!(row["verdicts"].is_array());
    assert!(row["rejections"].is_array());
}
