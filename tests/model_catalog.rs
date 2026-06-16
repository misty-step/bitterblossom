//! Model catalog guard contracts. The local gate must prove configured
//! Pi/OpenRouter ids against a checked-in fixture without live network access.

use std::fs;
use std::path::PathBuf;
use std::process::{Command, Output};

use bitterblossom::spec::{AuthClass, Plane, TriggerSpec};
use serde_json::Value;

fn root() -> PathBuf {
    PathBuf::from(env!("CARGO_MANIFEST_DIR"))
}

fn run_check(args: &[&str]) -> Output {
    Command::new(root().join("scripts/check-model-catalog.sh"))
        .current_dir(root())
        .args(args)
        .output()
        .unwrap()
}

fn parse_json(output: &Output) -> Value {
    serde_json::from_slice(&output.stdout).unwrap_or_else(|err| {
        panic!(
            "invalid json: {err}\nstdout:\n{}\nstderr:\n{}",
            String::from_utf8_lossy(&output.stdout),
            String::from_utf8_lossy(&output.stderr)
        )
    })
}

#[test]
fn live_catalog_fetch_does_not_put_openrouter_key_in_curl_argv() {
    let script = fs::read_to_string(root().join("scripts/check-model-catalog.sh")).unwrap();

    assert!(!script.contains("-H \"Authorization: Bearer $OPENROUTER_API_KEY\""));
    assert!(script.contains("| curl --config -"));
}

#[test]
fn model_catalog_watch_task_is_manual_and_scheduled_api_reflex() {
    let plane = Plane::load(&root().join("plane")).unwrap();
    let task = plane.task("model-catalog-watch").unwrap();

    assert_eq!(task.agent.harness, "pi");
    assert_eq!(task.agent.model, "deepseek/deepseek-v4-flash");
    assert_eq!(task.agent.auth_class().unwrap(), AuthClass::Api);
    assert_eq!(task.spec.substrate, "sprites");
    assert!(task
        .spec
        .triggers
        .iter()
        .any(|trigger| matches!(trigger, TriggerSpec::Manual)));
    assert!(task
        .spec
        .triggers
        .iter()
        .any(|trigger| matches!(trigger, TriggerSpec::Cron { .. })));

    for required in [
        "dry_run`: default `true`",
        "file_backlog_pr",
        "REPORT.json",
        "OpenRouter",
        "fixture_drift",
        "new_family_candidates",
        "configured_successors",
        "Do not edit `plane/agents/*.toml`",
        "model-eval record",
    ] {
        assert!(task.card.contains(required), "missing {required}");
    }
}

#[test]
fn fixture_catalog_validates_configured_openrouter_models() {
    let output = run_check(&[
        "--catalog",
        "tests/fixtures/openrouter-models-current.json",
        "--json",
    ]);

    assert!(
        output.status.success(),
        "stdout:\n{}\nstderr:\n{}",
        String::from_utf8_lossy(&output.stdout),
        String::from_utf8_lossy(&output.stderr)
    );

    let report = parse_json(&output);
    assert_eq!(report["status"], "pass");
    assert_eq!(report["missing"].as_array().unwrap().len(), 0);
    assert_eq!(report["metadata_gaps"].as_array().unwrap().len(), 0);
    assert_eq!(report["docs_missing"].as_array().unwrap().len(), 0);
    assert!(report["configured"]
        .as_array()
        .unwrap()
        .iter()
        .any(|model| {
            model["id"] == "z-ai/glm-5.2" && model["context_length"].as_u64() == Some(1_048_576)
        }));
    assert!(report["configured_successors"]
        .as_array()
        .unwrap()
        .iter()
        .any(|entry| {
            entry["agent_file"] == "plane/agents/review-coordinator.toml"
                && entry["current"]["id"] == "moonshotai/kimi-k2.6:minimal"
                && entry["current"]["catalog_id"] == "moonshotai/kimi-k2.6"
                && entry["successors"]
                    .as_array()
                    .unwrap()
                    .iter()
                    .any(|successor| successor["id"] == "moonshotai/kimi-k2.7-code")
        }));
}

#[test]
fn missing_configured_model_is_a_gate_failure() {
    let dir = tempfile::tempdir().unwrap();
    let agents = dir.path().join("agents");
    fs::create_dir(&agents).unwrap();
    fs::write(
        agents.join("bad-reviewer.toml"),
        r#"
version = 1
harness = "pi"
model = 'missing/provider-model'
"#,
    )
    .unwrap();
    let docs = dir.path().join("docs.md");
    fs::write(&docs, "missing/provider-model\n").unwrap();

    let output = run_check(&[
        "--catalog",
        "tests/fixtures/openrouter-models-current.json",
        "--agents",
        agents.to_str().unwrap(),
        "--docs",
        docs.to_str().unwrap(),
        "--json",
    ]);

    assert!(!output.status.success());
    assert_eq!(output.status.code(), Some(1));

    let report = parse_json(&output);
    assert_eq!(report["status"], "fail");
    assert_eq!(report["missing"][0]["id"], "missing/provider-model");
}

#[test]
fn malformed_catalog_metadata_is_a_gate_failure() {
    let dir = tempfile::tempdir().unwrap();
    let agents = dir.path().join("agents");
    fs::create_dir(&agents).unwrap();
    fs::write(
        agents.join("thin.toml"),
        r#"
version = 1
harness = "pi"
model = "provider/thin"
"#,
    )
    .unwrap();
    let docs = dir.path().join("docs.md");
    fs::write(&docs, "provider/thin\n").unwrap();
    let catalog = dir.path().join("catalog.json");
    fs::write(
        &catalog,
        r#"
{
  "data": [
    {
      "id": "provider/thin",
      "name": "Provider Thin"
    }
  ]
}
"#,
    )
    .unwrap();

    let output = run_check(&[
        "--catalog",
        catalog.to_str().unwrap(),
        "--agents",
        agents.to_str().unwrap(),
        "--docs",
        docs.to_str().unwrap(),
        "--json",
    ]);

    assert!(!output.status.success());
    assert_eq!(output.status.code(), Some(1));

    let report = parse_json(&output);
    assert_eq!(report["status"], "fail");
    assert_eq!(report["metadata_gaps"][0]["id"], "provider/thin");
    let fields = report["metadata_gaps"][0]["fields"].as_array().unwrap();
    assert!(fields
        .iter()
        .any(|field| field == "pricing.prompt" || field == "context_length"));
}
