use std::fs;
use std::os::unix::fs::PermissionsExt;
use std::process::Command;

use bitterblossom::harness::parse_output;

fn repo_root() -> std::path::PathBuf {
    std::path::PathBuf::from(env!("CARGO_MANIFEST_DIR"))
}

fn write_executable(path: &std::path::Path, content: &str) {
    fs::write(path, content).unwrap();
    fs::set_permissions(path, fs::Permissions::from_mode(0o755)).unwrap();
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
