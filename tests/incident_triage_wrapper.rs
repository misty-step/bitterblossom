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
  "incident": {{"id": "INC-test123", "service": "{service}", "severity": "low", "opened_at": "2026-07-02T00:00:00Z"}}
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
