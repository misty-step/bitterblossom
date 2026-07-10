use std::fs;
use std::io::{BufRead, BufReader, Read, Write};
use std::net::{TcpListener, TcpStream};
use std::os::unix::fs::PermissionsExt;
use std::process::Command;
use std::sync::{Arc, Mutex};
use std::thread;
use std::time::Duration;

use bitterblossom::harness::parse_output;

fn repo_root() -> std::path::PathBuf {
    std::path::PathBuf::from(env!("CARGO_MANIFEST_DIR"))
}

fn write_executable(path: &std::path::Path, content: &str) {
    fs::write(path, content).unwrap();
    fs::set_permissions(path, fs::Permissions::from_mode(0o755)).unwrap();
}

/// (method, path, body) per received request.
type RequestLog = Vec<(String, String, String)>;

/// Minimal single-threaded HTTP stub standing in for Canary. `handler` maps
/// (method, path, body) to (status, body) and every request is logged so
/// tests can assert on exact escalate-call shape (idempotency key, reason).
fn spawn_stub_canary(
    max_conns: usize,
    handler: impl Fn(&str, &str, &str) -> (u16, String) + Send + 'static,
) -> (u16, Arc<Mutex<RequestLog>>) {
    let listener = TcpListener::bind("127.0.0.1:0").unwrap();
    let port = listener.local_addr().unwrap().port();
    let log: Arc<Mutex<RequestLog>> = Arc::new(Mutex::new(Vec::new()));
    let log2 = log.clone();
    thread::spawn(move || {
        for stream in listener.incoming().take(max_conns) {
            let Ok(stream) = stream else { continue };
            handle_stub_conn(stream, &handler, &log);
        }
    });
    (port, log2)
}

fn handle_stub_conn(
    stream: TcpStream,
    handler: &(impl Fn(&str, &str, &str) -> (u16, String) + Send + 'static),
    log: &Arc<Mutex<RequestLog>>,
) {
    stream.set_read_timeout(Some(Duration::from_secs(5))).ok();
    let mut reader = BufReader::new(stream.try_clone().unwrap());
    let mut request_line = String::new();
    if reader.read_line(&mut request_line).unwrap_or(0) == 0 {
        return;
    }
    let mut parts = request_line.split_whitespace();
    let method = parts.next().unwrap_or("").to_string();
    let path = parts.next().unwrap_or("").to_string();
    let mut content_length = 0usize;
    loop {
        let mut line = String::new();
        if reader.read_line(&mut line).unwrap_or(0) == 0 {
            break;
        }
        if line == "\r\n" || line.is_empty() {
            break;
        }
        if let Some(v) = line.to_ascii_lowercase().strip_prefix("content-length:") {
            content_length = v.trim().parse().unwrap_or(0);
        }
    }
    let mut body = vec![0u8; content_length];
    if content_length > 0 {
        let _ = reader.read_exact(&mut body);
    }
    let body_str = String::from_utf8_lossy(&body).to_string();
    log.lock()
        .unwrap()
        .push((method.clone(), path.clone(), body_str.clone()));
    let (status, resp_body) = handler(&method, &path, &body_str);
    let response = format!(
        "HTTP/1.1 {status} x\r\nContent-Type: application/json\r\nContent-Length: {}\r\nConnection: close\r\n\r\n{}",
        resp_body.len(),
        resp_body
    );
    let mut stream = stream;
    let _ = stream.write_all(response.as_bytes());
}

fn git(repo: &std::path::Path, args: &[&str]) {
    let status = Command::new("git")
        .current_dir(repo)
        .args(args)
        .status()
        .unwrap();
    assert!(status.success(), "git {args:?} failed in {repo:?}");
}

/// A real (throwaway) git checkout with `bb/incident-<id>-attempt-<n>`
/// branches, mimicking the sprite workspace layout the wrapper inspects.
fn make_repo_with_attempt_branches(
    root: &std::path::Path,
    dir_name: &str,
    incident_id: &str,
    attempts: &[u32],
) -> std::path::PathBuf {
    let repo = root.join(dir_name);
    fs::create_dir_all(&repo).unwrap();
    git(&repo, &["init", "-q", "-b", "master"]);
    git(
        &repo,
        &[
            "-c",
            "user.name=t",
            "-c",
            "user.email=t@example.com",
            "commit",
            "-q",
            "--allow-empty",
            "-m",
            "init",
        ],
    );
    for n in attempts {
        git(
            &repo,
            &["branch", &format!("bb/incident-{incident_id}-attempt-{n}")],
        );
    }
    repo
}

fn write_event_and_run(dir: &std::path::Path, service: &str) {
    fs::write(
        dir.join("EVENT.json"),
        format!(
            r#"{{
  "schema_version": "canary.incident_event.v1",
  "event": "incident.opened",
  "subject": {{"type": "incident", "id": "INC-test123", "service": "{service}", "environment": "production"}},
  "signal": {{"kind": "error_group", "fingerprint": "fp-test", "severity": "low", "observed_at": "2026-07-02T00:00:00Z"}},
  "replay": {{"incident_url": "/api/v1/incidents/INC-test123", "timeline_url": "/api/v1/timeline?service={service}&window=1h", "report_url": "/api/v1/report?service={service}&window=1h"}},
  "incident": {{"id": "INC-test123", "service": "{service}", "severity": "low", "opened_at": "2026-07-02T00:00:00Z"}},
  "timestamp": "2026-07-02T00:00:10Z"
}}"#
        ),
    )
    .unwrap();
    fs::write(
        dir.join("RUN.json"),
        r#"{"run_id":"run-test","task":"incident-triage","trigger":{"kind":"webhook","idempotency_key":"wh:incident-triage:DLV-test"}}"#,
    )
    .unwrap();
    fs::write(dir.join("LANE_CARD.md"), "# card\n").unwrap();
}

#[test]
fn linejam_production_smoke_alert_creates_powder_needs_you_without_running_model() {
    let dir = tempfile::tempdir().unwrap();
    write_event_and_run(dir.path(), "linejam");
    let (canary_port, canary_log) = spawn_stub_canary(2, |method, path, _body| {
        if method == "GET" && path == "/api/v1/incidents/INC-test123" {
            return (
                200,
                r#"{"signals":[{"type":"health_transition","monitor_id":"MON-28junwbo5mgv","monitor_name":"linejam-production-smoke"}]}"#.to_string(),
            );
        }
        if method == "GET" && path.starts_with("/api/v1/annotations?") {
            return (
                200,
                r#"{"annotations":[{"created_at":"2026-07-02T00:00:08Z","metadata":{"kind":"production-smoke-status","outcome":"failure","consecutive_failures":1,"failure_detail":"tests/e2e/prod-smoke.spec.ts","external_url":"https://github.com/misty-step/linejam/actions/runs/12344"}},{"created_at":"2026-07-02T00:00:11Z","metadata":{"kind":"production-smoke-status","outcome":"success","consecutive_failures":0,"external_url":"https://github.com/misty-step/linejam/actions/runs/12346"}},{"created_at":"2026-07-02T00:00:09Z","metadata":{"kind":"production-smoke-status","outcome":"failure","consecutive_failures":2,"failure_detail":"tests/e2e/prod-smoke.spec.ts: signed-in user can join","external_url":"https://github.com/misty-step/linejam/actions/runs/12345"}}]}"#.to_string(),
            );
        }
        (404, "{}".to_string())
    });
    let (powder_port, powder_log) = spawn_stub_canary(4, |method, path, _body| {
        match (method, path) {
            ("GET", "/api/v1/cards/linejam-alert-inc-test123") => (404, "{}".to_string()),
            ("POST", "/api/v1/cards") => (
                200,
                r#"{"id":"linejam-alert-inc-test123","status":"ready"}"#.to_string(),
            ),
            ("POST", "/api/v1/cards/linejam-alert-inc-test123/claim") => (
                200,
                r#"{"card_id":"linejam-alert-inc-test123","run_id":"run-alert","expires_at":9999999999}"#
                    .to_string(),
            ),
            ("POST", "/api/v1/runs/run-alert/input") => (
                200,
                r#"{"id":"run-alert","card_id":"linejam-alert-12345","state":"awaiting_input"}"#
                    .to_string(),
            ),
            _ => (404, "{}".to_string()),
        }
    });

    let output = Command::new(repo_root().join("scripts/incident-triage-wrapper.sh"))
        .current_dir(dir.path())
        .env("CANARY_ENDPOINT", format!("http://127.0.0.1:{canary_port}"))
        .env("CANARY_API_KEY", "test-canary")
        .env(
            "POWDER_API_BASE_URL",
            format!("http://127.0.0.1:{powder_port}"),
        )
        .env("POWDER_INCIDENT_ALERT_API_KEY", "test-powder")
        .output()
        .unwrap();
    assert!(
        output.status.success(),
        "stdout={}\nstderr={}",
        String::from_utf8_lossy(&output.stdout),
        String::from_utf8_lossy(&output.stderr)
    );

    let report: serde_json::Value =
        serde_json::from_str(&fs::read_to_string(dir.path().join("REPORT.json")).unwrap()).unwrap();
    assert_eq!(report["status"], "operator_attention_requested");
    assert_eq!(report["repo"], "misty-step/linejam");
    assert_eq!(
        report["powder_alert"]["card_id"],
        "linejam-alert-inc-test123"
    );
    assert_eq!(report["powder_alert"]["run_id"], "run-alert");

    let requests = powder_log.lock().unwrap();
    let input = requests
        .iter()
        .find(|(method, path, _)| method == "POST" && path.ends_with("/input"))
        .expect("Powder request_input call");
    assert!(input.2.contains("signed-in user can join"));
    assert!(input.2.contains("actions/runs/12345"));
    drop(requests);
    assert_eq!(canary_log.lock().unwrap().len(), 2);
}

#[test]
fn linejam_production_smoke_single_failure_stays_below_page_threshold() {
    let dir = tempfile::tempdir().unwrap();
    write_event_and_run(dir.path(), "linejam");
    let (canary_port, _) = spawn_stub_canary(2, |method, path, _body| {
        if method == "GET" && path == "/api/v1/incidents/INC-test123" {
            return (
                200,
                r#"{"signals":[{"monitor_id":"MON-28junwbo5mgv","monitor_name":"linejam-production-smoke"}]}"#.to_string(),
            );
        }
        if method == "GET" && path.starts_with("/api/v1/annotations?") {
            return (
                200,
                r#"{"annotations":[{"created_at":"2026-07-02T00:00:09Z","metadata":{"kind":"production-smoke-status","outcome":"failure","consecutive_failures":1,"failure_detail":"tests/e2e/prod-smoke.spec.ts","external_url":"https://github.com/misty-step/linejam/actions/runs/12344"}}]}"#.to_string(),
            );
        }
        (404, "{}".to_string())
    });
    let output = Command::new(repo_root().join("scripts/incident-triage-wrapper.sh"))
        .current_dir(dir.path())
        .env("CANARY_ENDPOINT", format!("http://127.0.0.1:{canary_port}"))
        .env("CANARY_API_KEY", "test-canary")
        .output()
        .unwrap();
    assert!(output.status.success());
    let report: serde_json::Value =
        serde_json::from_str(&fs::read_to_string(dir.path().join("REPORT.json")).unwrap()).unwrap();
    assert_eq!(report["status"], "failure_below_threshold");
    assert!(report["powder_alert"]["card_id"].is_null());
}

#[test]
fn linejam_production_smoke_recovery_is_annotated_without_new_powder_input() {
    let dir = tempfile::tempdir().unwrap();
    write_event_and_run(dir.path(), "linejam");
    let mut event: serde_json::Value =
        serde_json::from_str(&fs::read_to_string(dir.path().join("EVENT.json")).unwrap()).unwrap();
    event["event"] = serde_json::json!("incident.resolved");
    event["timestamp"] = serde_json::json!("2026-07-02T00:00:20Z");
    fs::write(
        dir.path().join("EVENT.json"),
        serde_json::to_vec_pretty(&event).unwrap(),
    )
    .unwrap();
    let (canary_port, _) = spawn_stub_canary(2, |method, path, _body| {
        if method == "GET" && path == "/api/v1/incidents/INC-test123" {
            return (
                200,
                r#"{"signals":[{"monitor_id":"MON-28junwbo5mgv","monitor_name":"linejam-production-smoke","resolved_at":"2026-07-10T05:00:00Z"}]}"#.to_string(),
            );
        }
        if method == "GET" && path.starts_with("/api/v1/annotations?") {
            return (
                200,
                r#"{"annotations":[{"created_at":"2026-07-02T00:00:19Z","metadata":{"kind":"production-smoke-status","outcome":"success","external_url":"https://github.com/misty-step/linejam/actions/runs/12346"}}]}"#.to_string(),
            );
        }
        (404, "{}".to_string())
    });
    let (powder_port, powder_log) = spawn_stub_canary(3, |method, path, _body| {
        match (method, path) {
            ("GET", "/api/v1/cards/linejam-alert-inc-test123") => (
                200,
                r#"{"card":{"id":"linejam-alert-inc-test123","status":"awaiting_input","claim":{"agent":"bb-incident-triage-alert","run_id":"run-existing","acquired_at":1,"expires_at":9999999999}},"runs":[{"id":"run-existing","state":"awaiting_input"}]}"#.to_string(),
            ),
            ("POST", "/api/v1/runs/run-existing/answer") => (
                200,
                r#"{"id":"run-existing","state":"active"}"#.to_string(),
            ),
            ("POST", "/api/v1/cards/linejam-alert-inc-test123/complete") => (
                200,
                r#"{"id":"linejam-alert-inc-test123","status":"done"}"#.to_string(),
            ),
            _ => (404, "{}".to_string()),
        }
    });
    let output = Command::new(repo_root().join("scripts/incident-triage-wrapper.sh"))
        .current_dir(dir.path())
        .env("CANARY_ENDPOINT", format!("http://127.0.0.1:{canary_port}"))
        .env("CANARY_API_KEY", "test-canary")
        .env(
            "POWDER_API_BASE_URL",
            format!("http://127.0.0.1:{powder_port}"),
        )
        .env("POWDER_INCIDENT_ALERT_API_KEY", "test-powder")
        .output()
        .unwrap();
    assert!(output.status.success());
    let report: serde_json::Value =
        serde_json::from_str(&fs::read_to_string(dir.path().join("REPORT.json")).unwrap()).unwrap();
    assert_eq!(report["status"], "recovery_closed_operator_alert");
    assert_eq!(
        report["powder_alert"]["card_id"],
        "linejam-alert-inc-test123"
    );
    let requests = powder_log.lock().unwrap();
    assert_eq!(requests.len(), 3);
    assert!(requests.iter().any(|(method, path, body)| {
        method == "POST" && path.ends_with("/answer") && body.contains("Production Smoke recovered")
    }));
    assert!(requests
        .iter()
        .any(|(method, path, _)| { method == "POST" && path.ends_with("/complete") }));
}

#[test]
fn linejam_success_annotation_cannot_close_powder_before_resolved_event() {
    for event_name in ["incident.opened", "incident.updated"] {
        let dir = tempfile::tempdir().unwrap();
        write_event_and_run(dir.path(), "linejam");
        let mut event: serde_json::Value =
            serde_json::from_str(&fs::read_to_string(dir.path().join("EVENT.json")).unwrap())
                .unwrap();
        event["event"] = serde_json::json!(event_name);
        fs::write(
            dir.path().join("EVENT.json"),
            serde_json::to_vec_pretty(&event).unwrap(),
        )
        .unwrap();

        let (canary_port, _) = spawn_stub_canary(2, |method, path, _body| {
            if method == "GET" && path == "/api/v1/incidents/INC-test123" {
                return (
                    200,
                    r#"{"signals":[{"monitor_id":"MON-28junwbo5mgv","monitor_name":"linejam-production-smoke"}]}"#.to_string(),
                );
            }
            if method == "GET" && path.starts_with("/api/v1/annotations?") {
                return (
                    200,
                    r#"{"annotations":[{"created_at":"2026-07-02T00:00:09Z","metadata":{"kind":"production-smoke-status","outcome":"success","external_url":"https://github.com/misty-step/linejam/actions/runs/12346"}}]}"#.to_string(),
                );
            }
            (404, "{}".to_string())
        });
        let (powder_port, powder_log) =
            spawn_stub_canary(1, |_method, _path, _body| (500, "{}".to_string()));

        let output = Command::new(repo_root().join("scripts/incident-triage-wrapper.sh"))
            .current_dir(dir.path())
            .env("CANARY_ENDPOINT", format!("http://127.0.0.1:{canary_port}"))
            .env("CANARY_API_KEY", "test-canary")
            .env(
                "POWDER_API_BASE_URL",
                format!("http://127.0.0.1:{powder_port}"),
            )
            .env("POWDER_INCIDENT_ALERT_API_KEY", "test-powder")
            .output()
            .unwrap();
        assert!(
            output.status.success(),
            "event={event_name} stdout={} stderr={}",
            String::from_utf8_lossy(&output.stdout),
            String::from_utf8_lossy(&output.stderr)
        );

        let report: serde_json::Value =
            serde_json::from_str(&fs::read_to_string(dir.path().join("REPORT.json")).unwrap())
                .unwrap();
        assert_eq!(
            report["status"], "success_ignored_without_resolved_event",
            "event={event_name}"
        );
        assert!(
            powder_log.lock().unwrap().is_empty(),
            "event={event_name} must not read, answer, or complete Powder"
        );
    }
}

#[test]
fn linejam_production_smoke_redelivery_reuses_existing_powder_input() {
    let dir = tempfile::tempdir().unwrap();
    write_event_and_run(dir.path(), "linejam");
    let (canary_port, _) = spawn_stub_canary(2, |method, path, _body| {
        if method == "GET" && path == "/api/v1/incidents/INC-test123" {
            return (
                200,
                r#"{"signals":[{"monitor_id":"MON-28junwbo5mgv","monitor_name":"linejam-production-smoke"}]}"#.to_string(),
            );
        }
        if method == "GET" && path.starts_with("/api/v1/annotations?") {
            return (
                200,
                r#"{"annotations":[{"created_at":"2026-07-02T00:00:09Z","metadata":{"kind":"production-smoke-status","outcome":"failure","consecutive_failures":2,"failure_detail":"tests/e2e/prod-smoke.spec.ts: signed-in user can join","external_url":"https://github.com/misty-step/linejam/actions/runs/12345"}}]}"#.to_string(),
            );
        }
        (404, "{}".to_string())
    });
    let (powder_port, powder_log) = spawn_stub_canary(1, |method, path, _body| {
        if method == "GET" && path == "/api/v1/cards/linejam-alert-inc-test123" {
            return (
                200,
                r#"{"card":{"id":"linejam-alert-inc-test123","status":"awaiting_input","claim":{"agent":"bb-incident-triage-alert","run_id":"run-existing","acquired_at":1,"expires_at":9999999999}},"runs":[{"id":"run-existing","state":"awaiting_input"}]}"#.to_string(),
            );
        }
        (404, "{}".to_string())
    });

    let output = Command::new(repo_root().join("scripts/incident-triage-wrapper.sh"))
        .current_dir(dir.path())
        .env("CANARY_ENDPOINT", format!("http://127.0.0.1:{canary_port}"))
        .env("CANARY_API_KEY", "test-canary")
        .env(
            "POWDER_API_BASE_URL",
            format!("http://127.0.0.1:{powder_port}"),
        )
        .env("POWDER_INCIDENT_ALERT_API_KEY", "test-powder")
        .output()
        .unwrap();
    assert!(output.status.success());

    let report: serde_json::Value =
        serde_json::from_str(&fs::read_to_string(dir.path().join("REPORT.json")).unwrap()).unwrap();
    assert_eq!(report["status"], "operator_attention_already_requested");
    assert_eq!(report["powder_alert"]["run_id"], "run-existing");
    let requests = powder_log.lock().unwrap();
    assert_eq!(requests.len(), 1);
    assert_eq!(requests[0].0, "GET");
}

#[test]
fn linejam_production_smoke_resumes_same_actor_claim_after_interruption() {
    let dir = tempfile::tempdir().unwrap();
    write_event_and_run(dir.path(), "linejam");
    let (canary_port, _) = spawn_stub_canary(2, |method, path, _body| {
        if method == "GET" && path == "/api/v1/incidents/INC-test123" {
            return (
                200,
                r#"{"signals":[{"monitor_id":"MON-28junwbo5mgv","monitor_name":"linejam-production-smoke"}]}"#.to_string(),
            );
        }
        if method == "GET" && path.starts_with("/api/v1/annotations?") {
            return (
                200,
                r#"{"annotations":[{"created_at":"2026-07-02T00:00:09Z","metadata":{"kind":"production-smoke-status","outcome":"failure","consecutive_failures":2,"failure_detail":"tests/e2e/prod-smoke.spec.ts:42:7","external_url":"https://github.com/misty-step/linejam/actions/runs/12345"}}]}"#.to_string(),
            );
        }
        (404, "{}".to_string())
    });
    let (powder_port, powder_log) = spawn_stub_canary(3, |method, path, _body| {
        match (method, path) {
            ("GET", "/api/v1/cards/linejam-alert-inc-test123") => (
                200,
                r#"{"card":{"id":"linejam-alert-inc-test123","status":"claimed","claim":{"agent":"bb-incident-triage-alert","run_id":"run-interrupted","acquired_at":1,"expires_at":9999999999}},"runs":[{"id":"run-interrupted","state":"active"}]}"#.to_string(),
            ),
            ("POST", "/api/v1/cards/linejam-alert-inc-test123/claim") => (
                200,
                r#"{"card_id":"linejam-alert-inc-test123","run_id":"run-interrupted","agent":"bb-incident-triage-alert","expires_at":9999999999}"#.to_string(),
            ),
            ("POST", "/api/v1/runs/run-interrupted/input") => (
                200,
                r#"{"id":"run-interrupted","card_id":"linejam-alert-inc-test123","state":"awaiting_input"}"#.to_string(),
            ),
            _ => (404, "{}".to_string()),
        }
    });

    let output = Command::new(repo_root().join("scripts/incident-triage-wrapper.sh"))
        .current_dir(dir.path())
        .env("CANARY_ENDPOINT", format!("http://127.0.0.1:{canary_port}"))
        .env("CANARY_API_KEY", "test-canary")
        .env(
            "POWDER_API_BASE_URL",
            format!("http://127.0.0.1:{powder_port}"),
        )
        .env("POWDER_INCIDENT_ALERT_API_KEY", "test-powder")
        .output()
        .unwrap();
    assert!(output.status.success());
    let report: serde_json::Value =
        serde_json::from_str(&fs::read_to_string(dir.path().join("REPORT.json")).unwrap()).unwrap();
    assert_eq!(report["status"], "operator_attention_requested");
    assert_eq!(report["powder_alert"]["run_id"], "run-interrupted");
    let requests = powder_log.lock().unwrap();
    assert_eq!(requests.len(), 3);
    assert!(requests
        .iter()
        .any(|(method, path, _)| method == "POST" && path.ends_with("/input")));
}

#[test]
fn linejam_production_smoke_concurrent_create_reuses_peer_powder_input() {
    let dir = tempfile::tempdir().unwrap();
    write_event_and_run(dir.path(), "linejam");
    let (canary_port, _) = spawn_stub_canary(2, |method, path, _body| {
        if method == "GET" && path == "/api/v1/incidents/INC-test123" {
            return (
                200,
                r#"{"signals":[{"monitor_id":"MON-28junwbo5mgv","monitor_name":"linejam-production-smoke"}]}"#.to_string(),
            );
        }
        if method == "GET" && path.starts_with("/api/v1/annotations?") {
            return (
                200,
                r#"{"annotations":[{"created_at":"2026-07-02T00:00:09Z","metadata":{"kind":"production-smoke-status","outcome":"failure","consecutive_failures":2,"failure_detail":"tests/e2e/prod-smoke.spec.ts: signed-in user can join","external_url":"https://github.com/misty-step/linejam/actions/runs/12345"}}]}"#.to_string(),
            );
        }
        (404, "{}".to_string())
    });
    let powder_reads = Arc::new(Mutex::new(0usize));
    let reads = powder_reads.clone();
    let (powder_port, powder_log) = spawn_stub_canary(3, move |method, path, _body| {
        match (method, path) {
            ("GET", "/api/v1/cards/linejam-alert-inc-test123") => {
                let mut read_count = reads.lock().unwrap();
                *read_count += 1;
                if *read_count == 1 {
                    (404, "{}".to_string())
                } else {
                    (
                        200,
                        r#"{"card":{"id":"linejam-alert-inc-test123","status":"awaiting_input","claim":{"agent":"bb-incident-triage-alert","run_id":"run-peer","acquired_at":1,"expires_at":9999999999}},"runs":[{"id":"run-peer","state":"awaiting_input"}]}"#.to_string(),
                    )
                }
            }
            ("POST", "/api/v1/cards") => (409, r#"{"error":"card already exists"}"#.to_string()),
            _ => (404, "{}".to_string()),
        }
    });

    let output = Command::new(repo_root().join("scripts/incident-triage-wrapper.sh"))
        .current_dir(dir.path())
        .env("CANARY_ENDPOINT", format!("http://127.0.0.1:{canary_port}"))
        .env("CANARY_API_KEY", "test-canary")
        .env(
            "POWDER_API_BASE_URL",
            format!("http://127.0.0.1:{powder_port}"),
        )
        .env("POWDER_INCIDENT_ALERT_API_KEY", "test-powder")
        .output()
        .unwrap();
    assert!(output.status.success());
    let report: serde_json::Value =
        serde_json::from_str(&fs::read_to_string(dir.path().join("REPORT.json")).unwrap()).unwrap();
    assert_eq!(report["status"], "operator_attention_already_requested");
    assert_eq!(report["powder_alert"]["run_id"], "run-peer");
    assert_eq!(powder_log.lock().unwrap().len(), 3);
}

#[test]
fn incident_triage_wrapper_runs_model_and_emits_command_result() {
    let dir = tempfile::tempdir().unwrap();
    write_event_and_run(dir.path(), "canary");
    let stub = dir.path().join("triage-agent-stub.sh");
    write_executable(
        &stub,
        r#"#!/bin/sh
set -eu
prompt="$(cat)"
case "$prompt" in
  *"max_fix_attempts: 3"*|*"max_fix_attempts = 3"*) ;;
  *) echo "prompt missing max attempts" >&2; exit 2;;
esac
cat > REPORT.json <<'JSON'
{
  "schema": "bb.incident_triage_response.v1",
  "status": "hypotheses_written",
  "bb_run_id": "run-test",
  "delivery_id": "DLV-test",
  "incident": {
    "id": "INC-test123",
    "service": "canary",
    "severity": "low",
    "fingerprint": "fp-test"
  },
  "repo": "misty-step/canary",
  "progress_writebacks": [
    {"action": "hypotheses-written", "ref": "ANN-test"}
  ],
  "hypotheses": [
    {"claim": "synthetic low-severity drill", "confidence": "medium", "why": "fixture"}
  ],
  "experiments": [],
  "fix_attempts": [],
  "iteration_guard": {
    "max_fix_attempts": 3,
    "attempts_used": 0,
    "stopped": false
  },
  "scope_honesty": {
    "auto_deploy_on_merge": true,
    "v1_stop": "hypotheses_writeback_drill"
  },
  "artifact_paths": ["REPORT.json"],
  "residual_risk": []
}
JSON
printf '{"type":"turn_end"}\n'
printf '{"type":"message_end","message":{"role":"assistant","content":[{"type":"text","text":"done"}],"usage":{"input":10,"output":20,"cost":{"total":0.03}}}}\n'
"#,
    );

    let output = Command::new(repo_root().join("scripts/incident-triage-wrapper.sh"))
        .current_dir(dir.path())
        .env("INCIDENT_TRIAGE_AGENT_BIN", &stub)
        .env("OPENROUTER_API_KEY", "test-openrouter")
        .env("GH_TOKEN", "test-gh")
        .env("CANARY_ENDPOINT", "http://127.0.0.1:1")
        .env("CANARY_API_KEY", "test-canary")
        .output()
        .unwrap();
    assert!(
        output.status.success(),
        "stdout={}\nstderr={}",
        String::from_utf8_lossy(&output.stdout),
        String::from_utf8_lossy(&output.stderr)
    );

    let report: serde_json::Value =
        serde_json::from_str(&fs::read_to_string(dir.path().join("REPORT.json")).unwrap()).unwrap();
    assert_eq!(report["schema"], "bb.incident_triage_response.v1");
    assert_eq!(report["status"], "hypotheses_written");
    assert_eq!(report["repo"], "misty-step/canary");

    let parsed = parse_output("command", &String::from_utf8(output.stdout).unwrap()).unwrap();
    assert_eq!(
        parsed.result,
        "incident triage hypotheses_written for misty-step/canary INC-test123"
    );
    assert_eq!(parsed.stats.tokens_in, Some(10));
    assert_eq!(parsed.stats.tokens_out, Some(20));
    assert_eq!(parsed.stats.turns, Some(1));
    assert_eq!(parsed.stats.cost_usd, Some(0.03));
}

/// Regression for bitterblossom-918: the wrapper's real (PATH-found) `pi`
/// invocation must disable extension discovery, matching the systemic
/// `harness.rs::build_command` fix — a global pi extension (observed:
/// ops-watchdog) can crash a `--no-session` run after a successful model
/// response (reproduced live against this machine's real pi + ops-watchdog
/// install; see bitterblossom-918-report.md). Workaround pending the
/// upstream fix (pi-agent-config#23, deliberately unmerged per the
/// operator's pi-agent-config retirement ruling).
#[test]
fn incident_triage_wrapper_disables_pi_extensions_by_default() {
    let dir = tempfile::tempdir().unwrap();
    write_event_and_run(dir.path(), "canary");
    // `agent_bin` must resolve literally to "pi" (not a custom path) to
    // exercise the wrapper's `[ "$agent_bin" = "pi" ]` native branch — a
    // stub named anything else falls through to the untouched generic
    // fallback branch, matching how the existing npx-fallback test below
    // manipulates PATH rather than INCIDENT_TRIAGE_AGENT_BIN.
    let bin_dir = dir.path().join("bin");
    fs::create_dir(&bin_dir).unwrap();
    let stub = bin_dir.join("pi");
    write_executable(
        &stub,
        r#"#!/bin/sh
set -eu
printf '%s\n' "$*" > invoked-args.txt
cat >/dev/null
cat > REPORT.json <<'JSON'
{
  "schema": "bb.incident_triage_response.v1",
  "status": "hypotheses_written",
  "bb_run_id": "run-test",
  "delivery_id": "DLV-test",
  "incident": {"id": "INC-test123", "service": "canary", "severity": "low", "fingerprint": "fp-test"},
  "repo": "misty-step/canary",
  "progress_writebacks": [{"action": "hypotheses-written", "ref": "ANN-test"}],
  "hypotheses": [{"claim": "synthetic", "confidence": "medium", "why": "fixture"}],
  "experiments": [],
  "fix_attempts": [],
  "iteration_guard": {"max_fix_attempts": 3, "attempts_used": 0, "stopped": false},
  "scope_honesty": {"auto_deploy_on_merge": true, "v1_stop": "hypotheses_writeback_drill"},
  "artifact_paths": ["REPORT.json"],
  "residual_risk": []
}
JSON
printf '{"type":"turn_end"}\n'
"#,
    );

    let path = format!("{}:/usr/bin:/bin", bin_dir.to_string_lossy());
    let output = Command::new(repo_root().join("scripts/incident-triage-wrapper.sh"))
        .current_dir(dir.path())
        .env("PATH", path)
        .env("OPENROUTER_API_KEY", "test-openrouter")
        .env("GH_TOKEN", "test-gh")
        .env("CANARY_ENDPOINT", "http://127.0.0.1:1")
        .env("CANARY_API_KEY", "test-canary")
        .output()
        .unwrap();
    assert!(
        output.status.success(),
        "stdout={}\nstderr={}",
        String::from_utf8_lossy(&output.stdout),
        String::from_utf8_lossy(&output.stderr)
    );

    let invoked_args = fs::read_to_string(dir.path().join("invoked-args.txt")).unwrap();
    assert!(
        invoked_args.contains("--no-extensions"),
        "the wrapper's pi invocation must disable extension discovery by \
         default (bitterblossom-918): {invoked_args}"
    );
}

/// Same regression as above, for the pinned-`npx`-fallback branch: the flag
/// must be present there too, not only on the PATH-found `pi` path.
#[test]
fn incident_triage_wrapper_npx_fallback_disables_pi_extensions_by_default() {
    let dir = tempfile::tempdir().unwrap();
    write_event_and_run(dir.path(), "powder");
    let bin_dir = dir.path().join("bin");
    fs::create_dir(&bin_dir).unwrap();
    let fake_npx = bin_dir.join("npx");
    write_executable(
        &fake_npx,
        r#"#!/bin/sh
set -eu
printf '%s\n' "$*" > invoked-args.txt
shift 2
cat >/dev/null
cat > REPORT.json <<'JSON'
{
  "schema": "bb.incident_triage_response.v1",
  "status": "hypotheses_written",
  "bb_run_id": "run-test",
  "delivery_id": "DLV-test",
  "incident": {"id": "INC-test123", "service": "powder", "severity": "low", "fingerprint": "fp-test"},
  "repo": "misty-step/powder",
  "progress_writebacks": [{"action": "hypotheses-written", "ref": "ANN-test"}],
  "hypotheses": [{"claim": "synthetic", "confidence": "medium", "why": "fixture"}],
  "experiments": [],
  "fix_attempts": [],
  "iteration_guard": {"max_fix_attempts": 3, "attempts_used": 0, "stopped": false},
  "scope_honesty": {"auto_deploy_on_merge": false, "v1_stop": "hypotheses_writeback_drill"},
  "artifact_paths": ["REPORT.json"],
  "residual_risk": []
}
JSON
printf '{"type":"turn_end"}\n'
"#,
    );

    let path = format!("{}:/usr/bin:/bin", bin_dir.to_string_lossy());
    let output = Command::new(repo_root().join("scripts/incident-triage-wrapper.sh"))
        .current_dir(dir.path())
        .env("PATH", path)
        .env("OPENROUTER_API_KEY", "test-openrouter")
        .env("GH_TOKEN", "test-gh")
        .env("CANARY_ENDPOINT", "http://127.0.0.1:1")
        .env("CANARY_API_KEY", "test-canary")
        .output()
        .unwrap();
    assert!(
        output.status.success(),
        "stdout={}\nstderr={}",
        String::from_utf8_lossy(&output.stdout),
        String::from_utf8_lossy(&output.stderr)
    );

    let invoked_args = fs::read_to_string(dir.path().join("invoked-args.txt")).unwrap();
    assert!(
        invoked_args.contains("--no-extensions"),
        "the wrapper's npx-fallback pi invocation must disable extension \
         discovery by default (bitterblossom-918): {invoked_args}"
    );
}

#[test]
fn incident_triage_wrapper_falls_back_to_pinned_npx_pi_package() {
    let dir = tempfile::tempdir().unwrap();
    write_event_and_run(dir.path(), "powder");
    let bin_dir = dir.path().join("bin");
    fs::create_dir(&bin_dir).unwrap();
    let fake_npx = bin_dir.join("npx");
    write_executable(
        &fake_npx,
        r#"#!/bin/sh
set -eu
[ "$1" = "-y" ] || { echo "missing -y" >&2; exit 2; }
[ "$2" = "@earendil-works/pi-coding-agent@0.80.3" ] || { echo "package=$2" >&2; exit 2; }
shift 2
prompt="$(cat)"
case "$prompt" in
  *"repo: misty-step/powder"*) ;;
  *) echo "prompt missing powder repo" >&2; exit 2;;
esac
cat > REPORT.json <<'JSON'
{
  "schema": "bb.incident_triage_response.v1",
  "status": "hypotheses_written",
  "bb_run_id": "run-test",
  "delivery_id": "DLV-test",
  "incident": {
    "id": "INC-test123",
    "service": "powder",
    "severity": "low",
    "fingerprint": "fp-test"
  },
  "repo": "misty-step/powder",
  "progress_writebacks": [
    {"action": "hypotheses-written", "ref": "ANN-test"}
  ],
  "hypotheses": [
    {"claim": "synthetic low-severity drill", "confidence": "medium", "why": "fixture"}
  ],
  "experiments": [],
  "fix_attempts": [],
  "iteration_guard": {
    "max_fix_attempts": 3,
    "attempts_used": 0,
    "stopped": false
  },
  "scope_honesty": {
    "auto_deploy_on_merge": false,
    "v1_stop": "hypotheses_writeback_drill"
  },
  "artifact_paths": ["REPORT.json"],
  "residual_risk": []
}
JSON
printf '{"type":"turn_end"}\n'
printf '{"type":"message_end","message":{"role":"assistant","content":[{"type":"text","text":"done"}],"usage":{"input":7,"output":11,"cost":{"total":0.02}}}}\n'
"#,
    );

    let path = format!("{}:/usr/bin:/bin", bin_dir.to_string_lossy());
    let output = Command::new(repo_root().join("scripts/incident-triage-wrapper.sh"))
        .current_dir(dir.path())
        .env("PATH", path)
        .env("OPENROUTER_API_KEY", "test-openrouter")
        .env("GH_TOKEN", "test-gh")
        .env("CANARY_ENDPOINT", "http://127.0.0.1:1")
        .env("CANARY_API_KEY", "test-canary")
        .output()
        .unwrap();
    assert!(
        output.status.success(),
        "stdout={}\nstderr={}",
        String::from_utf8_lossy(&output.stdout),
        String::from_utf8_lossy(&output.stderr)
    );

    let parsed = parse_output("command", &String::from_utf8(output.stdout).unwrap()).unwrap();
    assert_eq!(
        parsed.result,
        "incident triage hypotheses_written for misty-step/powder INC-test123"
    );
    assert_eq!(parsed.stats.tokens_in, Some(7));
    assert_eq!(parsed.stats.tokens_out, Some(11));
    assert_eq!(parsed.stats.cost_usd, Some(0.02));
}

#[test]
fn incident_triage_wrapper_rejects_malformed_token_env_name() {
    let dir = tempfile::tempdir().unwrap();
    let output = Command::new(repo_root().join("scripts/incident-triage-wrapper.sh"))
        .current_dir(dir.path())
        .env(
            "INCIDENT_TRIAGE_GH_TOKEN_ENV",
            "GH_TOKEN}\"; touch injected; #",
        )
        .env("GH_TOKEN", "test-gh")
        .output()
        .unwrap();
    assert!(!output.status.success());
    assert!(
        !dir.path().join("injected").exists(),
        "injected shell syntax must never run"
    );
    let stderr = String::from_utf8_lossy(&output.stderr);
    assert!(
        stderr.contains("must be a valid environment variable name"),
        "stderr={stderr}"
    );
}

#[test]
fn incident_triage_wrapper_blocks_when_agent_exits_without_report() {
    let dir = tempfile::tempdir().unwrap();
    write_event_and_run(dir.path(), "canary");
    let stub = dir.path().join("triage-agent-stub.sh");
    write_executable(
        &stub,
        r#"#!/bin/sh
cat >/dev/null
echo "model failed before report" >&2
exit 9
"#,
    );

    let output = Command::new(repo_root().join("scripts/incident-triage-wrapper.sh"))
        .current_dir(dir.path())
        .env("INCIDENT_TRIAGE_AGENT_BIN", &stub)
        .env("OPENROUTER_API_KEY", "test-openrouter")
        .env("GH_TOKEN", "test-gh")
        .env("CANARY_ENDPOINT", "http://127.0.0.1:1")
        .env("CANARY_API_KEY", "test-canary")
        .output()
        .unwrap();
    assert!(
        output.status.success(),
        "stdout={}\nstderr={}",
        String::from_utf8_lossy(&output.stdout),
        String::from_utf8_lossy(&output.stderr)
    );

    let report: serde_json::Value =
        serde_json::from_str(&fs::read_to_string(dir.path().join("REPORT.json")).unwrap()).unwrap();
    assert_eq!(report["status"], "blocked");
    assert_eq!(
        report["residual_risk"][0],
        "agent command exited 9 before REPORT.json"
    );
    let parsed = parse_output("command", &String::from_utf8(output.stdout).unwrap()).unwrap();
    assert!(
        parsed
            .result
            .contains("incident triage blocked: agent command exited 9 before REPORT.json"),
        "{}",
        parsed.result
    );
}

#[test]
fn incident_triage_wrapper_preserves_existing_report_when_stdout_cap_trips_late() {
    let dir = tempfile::tempdir().unwrap();
    write_event_and_run(dir.path(), "canary");
    let stub = dir.path().join("triage-agent-stub.sh");
    write_executable(
        &stub,
        r#"#!/bin/sh
set -eu
cat >/dev/null
cat > REPORT.json <<'JSON'
{
  "schema": "bb.incident_triage_response.v1",
  "status": "no_fix_needed",
  "bb_run_id": "run-test",
  "delivery_id": "DLV-test",
  "incident": {"id": "INC-test123", "service": "canary", "severity": "low", "fingerprint": "fp-test"},
  "repo": "misty-step/canary",
  "progress_writebacks": [
    {"action": "investigation-started", "ref": "ANN-start"},
    {"action": "no-fix-needed", "ref": "ANN-terminal"}
  ],
  "hypotheses": [],
  "experiments": [],
  "fix_attempts": [],
  "iteration_guard": {"max_fix_attempts": 3, "attempts_used": 0, "stopped": false, "reason": null},
  "scope_honesty": {"auto_deploy_on_merge": true, "v1_stop": "canary_terminal_writeback"},
  "artifact_paths": ["REPORT.json"],
  "residual_risk": []
}
JSON
python3 - <<'PY'
import sys
sys.stdout.write("x" * 6000)
sys.stdout.flush()
PY
exit 9
"#,
    );

    let output = Command::new(repo_root().join("scripts/incident-triage-wrapper.sh"))
        .current_dir(dir.path())
        .env("INCIDENT_TRIAGE_AGENT_BIN", &stub)
        .env("INCIDENT_TRIAGE_AGENT_STDOUT_MAX_BYTES", "4096")
        .env("OPENROUTER_API_KEY", "test-openrouter")
        .env("GH_TOKEN", "test-gh")
        .env("CANARY_ENDPOINT", "http://127.0.0.1:1")
        .env("CANARY_API_KEY", "test-canary")
        .output()
        .unwrap();
    assert!(
        output.status.success(),
        "stdout={}\nstderr={}",
        String::from_utf8_lossy(&output.stdout),
        String::from_utf8_lossy(&output.stderr)
    );

    let report: serde_json::Value =
        serde_json::from_str(&fs::read_to_string(dir.path().join("REPORT.json")).unwrap()).unwrap();
    assert_eq!(report["status"], "no_fix_needed");
    assert_eq!(report["progress_writebacks"][1]["ref"], "ANN-terminal");
    assert_eq!(
        report["scope_honesty"]["v1_stop"],
        "canary_terminal_writeback"
    );
    assert_eq!(report["residual_risk"], serde_json::json!([]));

    let stdout_len = fs::metadata(dir.path().join("incident-triage/stdout.jsonl"))
        .unwrap()
        .len();
    assert_eq!(stdout_len, 4096);
    let stderr = fs::read_to_string(dir.path().join("incident-triage/stderr.txt")).unwrap();
    assert!(
        stderr.contains("agent stdout exceeded INCIDENT_TRIAGE_AGENT_STDOUT_MAX_BYTES=4096"),
        "stderr={stderr}"
    );

    let parsed = parse_output("command", &String::from_utf8(output.stdout).unwrap()).unwrap();
    assert_eq!(
        parsed.result,
        "incident triage no_fix_needed for misty-step/canary INC-test123"
    );
}

#[test]
fn incident_triage_wrapper_synthesizes_report_from_terminal_writeback_receipts() {
    let dir = tempfile::tempdir().unwrap();
    write_event_and_run(dir.path(), "canary");
    let stub = dir.path().join("triage-agent-stub.sh");
    write_executable(
        &stub,
        r#"#!/bin/sh
set -eu
cat >/dev/null
mkdir -p incident-triage/writebacks
cat > incident-triage/writebacks/001-started.json <<'JSON'
{"action":"investigation-started","ref":"ANN-start"}
JSON
cat > incident-triage/writebacks/002-no-fix-needed.json <<'JSON'
{"action":"no-fix-needed","ref":"ANN-terminal","terminal":true}
JSON
echo "late model failure after writebacks" >&2
exit 153
"#,
    );

    let output = Command::new(repo_root().join("scripts/incident-triage-wrapper.sh"))
        .current_dir(dir.path())
        .env("INCIDENT_TRIAGE_AGENT_BIN", &stub)
        .env("OPENROUTER_API_KEY", "test-openrouter")
        .env("GH_TOKEN", "test-gh")
        .env("CANARY_ENDPOINT", "http://127.0.0.1:1")
        .env("CANARY_API_KEY", "test-canary")
        .output()
        .unwrap();
    assert!(
        output.status.success(),
        "stdout={}\nstderr={}",
        String::from_utf8_lossy(&output.stdout),
        String::from_utf8_lossy(&output.stderr)
    );

    let report: serde_json::Value =
        serde_json::from_str(&fs::read_to_string(dir.path().join("REPORT.json")).unwrap()).unwrap();
    assert_eq!(report["status"], "canary_writebacks_preserved");
    assert_eq!(report["progress_writebacks"].as_array().unwrap().len(), 2);
    assert_eq!(report["progress_writebacks"][1]["ref"], "ANN-terminal");
    assert_eq!(
        report["scope_honesty"]["v1_stop"],
        "agent_failed_after_canary_terminal_writebacks"
    );
    assert_ne!(report["scope_honesty"]["v1_stop"], "blocked_before_agent");
    assert!(report["residual_risk"][0]
        .as_str()
        .unwrap()
        .contains("agent command exited 153 before REPORT.json"));

    let parsed = parse_output("command", &String::from_utf8(output.stdout).unwrap()).unwrap();
    assert_eq!(
        parsed.result,
        "incident triage canary_writebacks_preserved for misty-step/canary INC-test123"
    );
}

#[test]
fn incident_triage_wrapper_uses_byte_counted_stdout_cap_not_ulimit() {
    let script =
        fs::read_to_string(repo_root().join("scripts/incident-triage-wrapper.sh")).unwrap();
    assert!(
        !script.contains("ulimit -f"),
        "stdout cap must be enforced by byte-counting the subprocess stream"
    );
}

#[test]
fn incident_triage_wrapper_blocks_unlisted_service_before_model_run() {
    let dir = tempfile::tempdir().unwrap();
    write_event_and_run(dir.path(), "elsewhere");
    let marker = dir.path().join("model-ran");
    let stub = dir.path().join("triage-agent-stub.sh");
    write_executable(
        &stub,
        &format!("#!/bin/sh\ntouch {}\nexit 9\n", marker.display()),
    );

    let output = Command::new(repo_root().join("scripts/incident-triage-wrapper.sh"))
        .current_dir(dir.path())
        .env("INCIDENT_TRIAGE_AGENT_BIN", &stub)
        .output()
        .unwrap();
    assert!(
        output.status.success(),
        "stdout={}\nstderr={}",
        String::from_utf8_lossy(&output.stdout),
        String::from_utf8_lossy(&output.stderr)
    );
    assert!(
        !marker.exists(),
        "model should not run for unlisted services"
    );

    let report: serde_json::Value =
        serde_json::from_str(&fs::read_to_string(dir.path().join("REPORT.json")).unwrap()).unwrap();
    assert_eq!(report["status"], "blocked");
    assert_eq!(
        report["residual_risk"][0],
        "service is not in the incident-triage whitelist"
    );
    let parsed = parse_output("command", &String::from_utf8(output.stdout).unwrap()).unwrap();
    assert!(parsed.result.contains("incident triage blocked"));
}

#[test]
fn incident_triage_wrapper_blocks_non_linejam_resolved_event_before_model_run() {
    let dir = tempfile::tempdir().unwrap();
    write_event_and_run(dir.path(), "canary");
    let mut event: serde_json::Value =
        serde_json::from_str(&fs::read_to_string(dir.path().join("EVENT.json")).unwrap()).unwrap();
    event["event"] = serde_json::json!("incident.resolved");
    fs::write(
        dir.path().join("EVENT.json"),
        serde_json::to_vec_pretty(&event).unwrap(),
    )
    .unwrap();
    let marker = dir.path().join("model-ran");
    let stub = dir.path().join("triage-agent-stub.sh");
    write_executable(
        &stub,
        &format!("#!/bin/sh\ntouch {}\nexit 9\n", marker.display()),
    );

    let output = Command::new(repo_root().join("scripts/incident-triage-wrapper.sh"))
        .current_dir(dir.path())
        .env("INCIDENT_TRIAGE_AGENT_BIN", &stub)
        .env("OPENROUTER_API_KEY", "test-openrouter")
        .env("GH_TOKEN", "test-gh")
        .env("CANARY_ENDPOINT", "http://127.0.0.1:1")
        .env("CANARY_API_KEY", "test-canary")
        .output()
        .unwrap();
    assert!(output.status.success());
    assert!(
        !marker.exists(),
        "resolved Canary incident must not run model"
    );
    let report: serde_json::Value =
        serde_json::from_str(&fs::read_to_string(dir.path().join("REPORT.json")).unwrap()).unwrap();
    assert_eq!(report["status"], "blocked");
    assert_eq!(
        report["residual_risk"][0],
        "incident.resolved is admitted only for the Linejam alert recovery path"
    );
}

#[test]
fn incident_triage_wrapper_skips_already_escalated_incident_before_model_run() {
    let dir = tempfile::tempdir().unwrap();
    write_event_and_run(dir.path(), "canary");
    let marker = dir.path().join("model-ran");
    let stub = dir.path().join("triage-agent-stub.sh");
    write_executable(
        &stub,
        &format!("#!/bin/sh\ntouch {}\nexit 9\n", marker.display()),
    );

    let (port, requests) = spawn_stub_canary(1, |_method, _path, _body| {
        (
            200,
            r#"{"incident":{"id":"INC-test123","escalated_at":"2026-07-02T00:00:00Z"}}"#
                .to_string(),
        )
    });

    let output = Command::new(repo_root().join("scripts/incident-triage-wrapper.sh"))
        .current_dir(dir.path())
        .env("INCIDENT_TRIAGE_AGENT_BIN", &stub)
        .env("OPENROUTER_API_KEY", "test-openrouter")
        .env("GH_TOKEN", "test-gh")
        .env("CANARY_ENDPOINT", format!("http://127.0.0.1:{port}"))
        .env("CANARY_API_KEY", "test-canary")
        .output()
        .unwrap();
    assert!(
        output.status.success(),
        "stdout={}\nstderr={}",
        String::from_utf8_lossy(&output.stdout),
        String::from_utf8_lossy(&output.stderr)
    );
    assert!(
        !marker.exists(),
        "model should never run for an already-escalated incident"
    );

    let report: serde_json::Value =
        serde_json::from_str(&fs::read_to_string(dir.path().join("REPORT.json")).unwrap()).unwrap();
    assert_eq!(report["status"], "skipped_escalated");
    assert_eq!(report["escalation"]["escalated"], true);

    let seen = requests.lock().unwrap();
    assert_eq!(seen.len(), 1);
    assert_eq!(seen[0].0, "GET");
    assert_eq!(seen[0].1, "/api/v1/incidents/INC-test123");

    let parsed = parse_output("command", &String::from_utf8(output.stdout).unwrap()).unwrap();
    assert!(
        parsed.result.contains("incident triage skipped"),
        "{}",
        parsed.result
    );
}

#[test]
fn incident_triage_wrapper_backstops_escalation_when_report_omits_the_call() {
    let dir = tempfile::tempdir().unwrap();
    write_event_and_run(dir.path(), "canary");
    let stub = dir.path().join("triage-agent-stub.sh");
    write_executable(
        &stub,
        r#"#!/bin/sh
set -eu
cat >/dev/null
cat > REPORT.json <<'JSON'
{
  "schema": "bb.incident_triage_response.v1",
  "status": "escalation_needed",
  "bb_run_id": "run-test",
  "delivery_id": "DLV-test",
  "incident": {"id": "INC-test123", "service": "canary", "severity": "low", "fingerprint": "fp-test"},
  "repo": "misty-step/canary",
  "progress_writebacks": [],
  "hypotheses": [],
  "experiments": [],
  "fix_attempts": [],
  "iteration_guard": {"max_fix_attempts": 3, "attempts_used": 3, "stopped": true, "reason": "exhausted verification chain"},
  "scope_honesty": {"auto_deploy_on_merge": true, "v1_stop": "blocked"},
  "artifact_paths": ["REPORT.json"],
  "residual_risk": []
}
JSON
printf '{"type":"turn_end"}\n'
"#,
    );

    let (port, requests) = spawn_stub_canary(2, |method, path, _body| {
        if method == "GET" {
            (200, r#"{"incident":{"id":"INC-test123"}}"#.to_string())
        } else {
            (
                200,
                format!(
                    r#"{{"escalation":{{"incident_id":"INC-test123","escalated_at":"2026-07-02T01:00:00Z","escalated_by":"bitterblossom/canary-triage","path":"{path}"}}}}"#
                ),
            )
        }
    });

    let output = Command::new(repo_root().join("scripts/incident-triage-wrapper.sh"))
        .current_dir(dir.path())
        .env("INCIDENT_TRIAGE_AGENT_BIN", &stub)
        .env("OPENROUTER_API_KEY", "test-openrouter")
        .env("GH_TOKEN", "test-gh")
        .env("CANARY_ENDPOINT", format!("http://127.0.0.1:{port}"))
        .env("CANARY_API_KEY", "test-canary")
        .output()
        .unwrap();
    assert!(
        output.status.success(),
        "stdout={}\nstderr={}",
        String::from_utf8_lossy(&output.stdout),
        String::from_utf8_lossy(&output.stderr)
    );

    let report: serde_json::Value =
        serde_json::from_str(&fs::read_to_string(dir.path().join("REPORT.json")).unwrap()).unwrap();
    assert_eq!(report["escalation"]["escalated"], true);
    assert_eq!(
        report["escalation"]["response"]["escalation"]["escalated_by"],
        "bitterblossom/canary-triage"
    );

    let seen = requests.lock().unwrap();
    let escalate_call = seen
        .iter()
        .find(|(method, path, _)| method == "POST" && path.ends_with("/escalate"))
        .expect("wrapper must call /escalate as a backstop when the report skipped it");
    assert_eq!(escalate_call.1, "/api/v1/incidents/INC-test123/escalate");
    let body: serde_json::Value = serde_json::from_str(&escalate_call.2).unwrap();
    assert_eq!(body["owner"], "bitterblossom/canary-triage");
    assert_eq!(body["purpose"], "triage_escalation");
    assert_eq!(
        body["idempotency_key"],
        "bb-run-run-test:INC-test123:escalate"
    );
}

#[test]
fn incident_triage_wrapper_fails_hard_when_repo_branches_exceed_max_fix_attempts() {
    let dir = tempfile::tempdir().unwrap();
    write_event_and_run(dir.path(), "canary");
    make_repo_with_attempt_branches(dir.path(), "canary", "INC-test123", &[1, 2, 3, 4]);
    let stub = dir.path().join("triage-agent-stub.sh");
    write_executable(
        &stub,
        r#"#!/bin/sh
set -eu
cat >/dev/null
cat > REPORT.json <<'JSON'
{
  "schema": "bb.incident_triage_response.v1",
  "status": "pr_opened",
  "bb_run_id": "run-test",
  "delivery_id": "DLV-test",
  "incident": {"id": "INC-test123", "service": "canary", "severity": "low", "fingerprint": "fp-test"},
  "repo": "misty-step/canary",
  "progress_writebacks": [],
  "hypotheses": [],
  "experiments": [],
  "fix_attempts": [],
  "iteration_guard": {"max_fix_attempts": 3, "attempts_used": 1, "stopped": false, "reason": null},
  "scope_honesty": {"auto_deploy_on_merge": true, "v1_stop": "blocked"},
  "artifact_paths": ["REPORT.json"],
  "residual_risk": []
}
JSON
printf '{"type":"turn_end"}\n'
"#,
    );

    let output = Command::new(repo_root().join("scripts/incident-triage-wrapper.sh"))
        .current_dir(dir.path())
        .env("INCIDENT_TRIAGE_AGENT_BIN", &stub)
        .env("OPENROUTER_API_KEY", "test-openrouter")
        .env("GH_TOKEN", "test-gh")
        .env("CANARY_ENDPOINT", "http://127.0.0.1:1")
        .env("CANARY_API_KEY", "test-canary")
        .output()
        .unwrap();
    assert!(
        !output.status.success(),
        "wrapper must fail the run when the checked-out repo proves the guard was violated \
         even though the report under-claims attempts_used=1; stdout={}\nstderr={}",
        String::from_utf8_lossy(&output.stdout),
        String::from_utf8_lossy(&output.stderr)
    );
    let stderr = String::from_utf8_lossy(&output.stderr);
    assert!(
        stderr.contains("iteration guard violated"),
        "stderr={stderr}"
    );

    let report: serde_json::Value =
        serde_json::from_str(&fs::read_to_string(dir.path().join("REPORT.json")).unwrap()).unwrap();
    assert_eq!(report["iteration_guard"]["attempts_used"], 4);
}
