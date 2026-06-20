use std::fs;
use std::os::unix::fs::PermissionsExt;
use std::path::Path;
use std::process::Command;

use serde_json::Value;

const CLAUDE_STUB: &str = r#"#!/bin/sh
cat > /dev/null
printf '{"status":"ok","artifact_paths":["REPORT.json"]}\n' > REPORT.json
echo '{"type":"result","subtype":"success","result":"export me","total_cost_usd":0.0123,"num_turns":3,"usage":{"input_tokens":120,"output_tokens":45}}'
"#;

const CERBERUS_REVIEW_ARTIFACT_STUB: &str = r#"#!/bin/sh
cat > /dev/null
cat > REPORT.json <<'JSON'
{
  "schema_version": "review-run-artifact.v1",
  "run_id": "bb-cerberus-review",
  "request_id": "bb-cerberus-review",
  "request_digest": "bb-request-digest",
  "config_digest": "bb-config-digest",
  "reviewed_head_sha": "bb-head-sha",
  "pre_override_verdict": "PASS",
  "verdict": "PASS",
  "summary": "Bitterblossom carried a Cerberus review artifact as run evidence.",
  "findings": [],
  "reviewer_artifacts": [
    {
      "schema_version": "reviewer-artifact.v1",
      "reviewer_id": "sentinel",
      "perspective": "contract storage",
      "model": "fixture",
      "status": "completed",
      "verdict": "PASS",
      "summary": "No storage issue found.",
      "findings": [],
      "coverage": {
        "files_reviewed": ["tasks/demo/card.md"],
        "files_with_findings": []
      },
      "usage": {
        "prompt_tokens": 0,
        "completion_tokens": 0
      },
      "cost_usd": 0.0,
      "degraded_reason": null
    }
  ],
  "stats": {
    "total": 1,
    "pass": 1,
    "warn": 0,
    "fail": 0,
    "skip": 0
  },
  "coverage": {
    "files_reviewed": ["tasks/demo/card.md"],
    "files_with_findings": []
  },
  "degraded": false,
  "reserves": [],
  "override_applied": null,
  "cost": {
    "total_usd": 0.0,
    "per_reviewer": {
      "sentinel": 0.0
    }
  }
}
JSON
echo '{"type":"result","subtype":"success","result":"cerberus artifact stored","total_cost_usd":0.0,"num_turns":1,"usage":{"input_tokens":1,"output_tokens":1}}'
"#;

fn write_executable(path: &Path, content: &str) {
    fs::write(path, content).unwrap();
    fs::set_permissions(path, fs::Permissions::from_mode(0o755)).unwrap();
}

fn write_plane(root: &Path) {
    write_plane_with_stub(root, CLAUDE_STUB, "");
}

fn write_plane_with_stub(root: &Path, stub: &str, task_extra: &str) {
    fs::create_dir_all(root.join("agents")).unwrap();
    fs::create_dir_all(root.join("tasks/demo")).unwrap();
    fs::write(root.join("plane.toml"), "dev = true\n").unwrap();
    let stub_path = root.join("stub-harness.sh");
    write_executable(&stub_path, stub);
    fs::write(
        root.join("agents/stub.toml"),
        format!(
            "version = 2\nharness = \"claude\"\nprovider = \"anthropic\"\nmodel = \"claude-fable-5\"\nbin = \"{}\"\n",
            stub_path.display()
        ),
    )
    .unwrap();
    fs::write(
        root.join("tasks/demo/card.md"),
        "# Demo\nExport telemetry.\n",
    )
    .unwrap();
    fs::write(
        root.join("tasks/demo/task.toml"),
        format!(
            "agent = \"stub\"\nsubstrate = \"local\"\n{}[[trigger]]\nkind = \"manual\"\n",
            task_extra
        ),
    )
    .unwrap();
}

fn bb(config: &Path, args: &[&str]) -> std::process::Output {
    Command::new(env!("CARGO_BIN_EXE_bb"))
        .arg("--config")
        .arg(config)
        .args(args)
        .output()
        .unwrap()
}

fn assert_v1(doc: &Value) {
    assert_eq!(doc["schema"], "bb.run_telemetry.v1");
    assert_eq!(doc["schema_version"], 1);
    assert!(doc["exported_at"].as_str().unwrap().ends_with('Z'));
    assert!(doc["run"]["id"].is_string());
    assert!(doc["run"]["task"].is_string());
    assert!(doc["run"]["trigger"]["kind"].is_string());
    assert!(doc["run"]["tokens"].is_object());
    assert!(!doc["attempts"].as_array().unwrap().is_empty());
    assert!(doc["retry"]["attempt_count"].as_i64().unwrap() >= 1);
    assert!(doc["dead_letter"]["status"].is_string());
    assert!(doc["artifacts"].is_array());
    assert_eq!(doc["daedalus"]["source"], "bitterblossom");
    assert!(doc["daedalus"]["agent_configs"].is_array());
    assert!(doc["otel"]["spans"].is_array());
    assert!(doc["otel"]["metrics"].is_array());
}

#[test]
fn fixture_is_the_v1_compatibility_contract() {
    for line in include_str!("fixtures/run-telemetry-v1.jsonl").lines() {
        let doc: Value = serde_json::from_str(line).unwrap();
        assert_v1(&doc);
        assert_eq!(doc["dead_letter"]["status"], "open");
        assert_eq!(doc["attempts"][0]["agent"]["provider"], "openrouter");
        assert_eq!(
            doc["otel"]["spans"][0]["attributes"]["gen_ai.usage.input_tokens"],
            300
        );
    }
}

#[test]
fn runs_export_emits_versioned_telemetry_jsonl() {
    let dir = tempfile::tempdir().unwrap();
    write_plane(dir.path());

    let run = bb(dir.path(), &["run", "demo", "--json"]);
    assert!(
        run.status.success(),
        "stdout:\n{}\nstderr:\n{}",
        String::from_utf8_lossy(&run.stdout),
        String::from_utf8_lossy(&run.stderr)
    );

    let export = bb(dir.path(), &["runs", "export"]);
    assert!(
        export.status.success(),
        "stdout:\n{}\nstderr:\n{}",
        String::from_utf8_lossy(&export.stdout),
        String::from_utf8_lossy(&export.stderr)
    );
    let line = String::from_utf8(export.stdout).unwrap();
    let doc: Value = serde_json::from_str(line.lines().next().unwrap()).unwrap();
    assert_v1(&doc);
    assert_eq!(doc["run"]["task"], "demo");
    assert_eq!(doc["run"]["state"], "success");
    assert_eq!(doc["run"]["agent"]["name"], "stub");
    assert_eq!(doc["run"]["agent"]["version"], 2);
    assert_eq!(doc["run"]["tokens"]["input"], 120);
    assert_eq!(doc["run"]["tokens"]["output"], 45);
    assert_eq!(doc["attempts"][0]["agent"]["harness"], "claude");
    assert_eq!(doc["attempts"][0]["agent"]["provider"], "anthropic");
    assert_eq!(doc["retry"]["mechanical_retry_count"], 0);
    assert_eq!(doc["dead_letter"]["status"], "none");
    assert_eq!(doc["artifacts"][0]["kind"], "attempt_artifact_dir");
    assert_eq!(
        doc["otel"]["spans"][0]["attributes"]["gen_ai.agent.version"],
        "2"
    );
}

#[test]
fn runs_export_carries_cerberus_review_artifact_report() {
    let dir = tempfile::tempdir().unwrap();
    write_plane_with_stub(
        dir.path(),
        CERBERUS_REVIEW_ARTIFACT_STUB,
        "required_artifacts = [\"REPORT.json\"]\n",
    );

    let run = bb(dir.path(), &["run", "demo", "--json"]);
    assert!(
        run.status.success(),
        "stdout:\n{}\nstderr:\n{}",
        String::from_utf8_lossy(&run.stdout),
        String::from_utf8_lossy(&run.stderr)
    );

    let export = bb(dir.path(), &["runs", "export"]);
    assert!(
        export.status.success(),
        "stdout:\n{}\nstderr:\n{}",
        String::from_utf8_lossy(&export.stdout),
        String::from_utf8_lossy(&export.stderr)
    );
    let line = String::from_utf8(export.stdout).unwrap();
    let doc: Value = serde_json::from_str(line.lines().next().unwrap()).unwrap();
    assert_v1(&doc);

    let artifact_dir = doc["artifacts"]
        .as_array()
        .unwrap()
        .iter()
        .find(|artifact| artifact["kind"] == "attempt_artifact_dir")
        .and_then(|artifact| artifact["path"].as_str())
        .unwrap();
    let report = fs::read_to_string(Path::new(artifact_dir).join("REPORT.json")).unwrap();
    let artifact: Value = serde_json::from_str(&report).unwrap();
    assert_eq!(artifact["schema_version"], "review-run-artifact.v1");
    assert_eq!(artifact["request_id"], "bb-cerberus-review");
    assert_eq!(artifact["reviewed_head_sha"], "bb-head-sha");
    assert!(artifact["reviewer_artifacts"].is_array());
    assert!(!report.to_lowercase().contains("olympus"));
}
