use std::fs;
use std::os::unix::fs::PermissionsExt;
use std::process::Command;

use bitterblossom::dispatch;
use bitterblossom::ledger::{IngressRequest, Ledger};
use bitterblossom::spec::Plane;

fn write_executable(path: &std::path::Path, content: &str) {
    fs::write(path, content).unwrap();
    fs::set_permissions(path, fs::Permissions::from_mode(0o755)).unwrap();
}

fn bb(root: &str, args: &[&str]) -> std::process::Output {
    Command::new(env!("CARGO_BIN_EXE_bb"))
        .args(["--config", root])
        .args(args)
        .output()
        .unwrap()
}

fn mkdirs(root: &std::path::Path, tasks: &[&str]) {
    fs::create_dir_all(root.join("agents")).unwrap();
    for t in tasks {
        fs::create_dir_all(root.join("tasks").join(t)).unwrap();
        fs::write(root.join("tasks").join(t).join("card.md"), "card\n").unwrap();
    }
}

const CLAUDE_STUB: &str = r#"#!/bin/sh
cat > /dev/null
printf '{"status":"ok","artifact_paths":["REPORT.json"]}\n' > REPORT.json
echo '{"type":"result","subtype":"success","result":"commission complete","total_cost_usd":0.0123,"num_turns":3,"usage":{"input_tokens":120,"output_tokens":45}}'
"#;

fn ingest_manual(ledger: &mut Ledger, task: &str) -> String {
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

#[test]
fn preflight_reports_missing_secret() {
    let dir = tempfile::tempdir().unwrap();
    let root_path = dir.path();
    mkdirs(root_path, &["demo"]);
    fs::write(root_path.join("plane.toml"), "dev = true\n").unwrap();
    let stub = root_path.join("stub.sh");
    write_executable(&stub, "#!/bin/sh\ncat > /dev/null\n");
    fs::write(
        root_path.join("agents/stub.toml"),
        format!(
            "version = 1\nharness = \"command\"\nmodel = \"\"\nbin = \"{}\"\nsecrets = [\"BB_PREFLIGHT_TEST_MISSING_TOKEN\"]\n",
            stub.display()
        ),
    )
    .unwrap();
    fs::write(
        root_path.join("tasks/demo/task.toml"),
        "agent = \"stub\"\nsubstrate = \"local\"\n[[trigger]]\nkind = \"manual\"\n",
    )
    .unwrap();

    let out = Command::new(env!("CARGO_BIN_EXE_bb"))
        .args([
            "--config",
            root_path.to_str().unwrap(),
            "preflight",
            "demo",
            "--json",
        ])
        .env_remove("BB_PREFLIGHT_TEST_MISSING_TOKEN")
        .output()
        .unwrap();
    assert!(
        !out.status.success(),
        "preflight with findings must exit non-zero"
    );
    let doc: serde_json::Value = serde_json::from_slice(&out.stdout).unwrap();
    assert!(doc["tasks_checked"]
        .as_array()
        .unwrap()
        .contains(&serde_json::json!("demo")));
    let finding = doc["findings"]
        .as_array()
        .unwrap()
        .iter()
        .find(|f| f["kind"] == "missing_secret")
        .expect("missing_secret finding");
    assert!(finding["detail"]
        .as_str()
        .unwrap()
        .contains("BB_PREFLIGHT_TEST_MISSING_TOKEN"));
}

#[test]
fn preflight_reports_blank_secret() {
    let dir = tempfile::tempdir().unwrap();
    let root_path = dir.path();
    mkdirs(root_path, &["demo"]);
    fs::write(root_path.join("plane.toml"), "dev = true\n").unwrap();
    let stub = root_path.join("stub.sh");
    write_executable(&stub, "#!/bin/sh\ncat > /dev/null\n");
    fs::write(
        root_path.join("agents/stub.toml"),
        format!(
            "version = 1\nharness = \"command\"\nmodel = \"\"\nbin = \"{}\"\nsecrets = [\"BB_PREFLIGHT_TEST_BLANK_TOKEN\"]\n",
            stub.display()
        ),
    )
    .unwrap();
    fs::write(
        root_path.join("tasks/demo/task.toml"),
        "agent = \"stub\"\nsubstrate = \"local\"\n[[trigger]]\nkind = \"manual\"\n",
    )
    .unwrap();

    let out = Command::new(env!("CARGO_BIN_EXE_bb"))
        .args([
            "--config",
            root_path.to_str().unwrap(),
            "preflight",
            "demo",
            "--json",
        ])
        .env("BB_PREFLIGHT_TEST_BLANK_TOKEN", "  ")
        .output()
        .unwrap();
    assert!(
        !out.status.success(),
        "preflight with a blank declared secret must exit non-zero"
    );
    let doc: serde_json::Value = serde_json::from_slice(&out.stdout).unwrap();
    assert!(doc["findings"]
        .as_array()
        .unwrap()
        .iter()
        .any(|f| f["kind"] == "missing_secret"
            && f["detail"]
                .as_str()
                .unwrap()
                .contains("BB_PREFLIGHT_TEST_BLANK_TOKEN")));
}

#[test]
fn preflight_reports_unspawnable_command_binary() {
    let dir = tempfile::tempdir().unwrap();
    let root_path = dir.path();
    mkdirs(root_path, &["demo"]);
    fs::write(root_path.join("plane.toml"), "dev = true\n").unwrap();
    fs::write(
        root_path.join("agents/stub.toml"),
        "version = 1\nharness = \"command\"\nmodel = \"\"\nbin = \"/no/such/binary/here\"\n",
    )
    .unwrap();
    fs::write(
        root_path.join("tasks/demo/task.toml"),
        "agent = \"stub\"\nsubstrate = \"local\"\n[[trigger]]\nkind = \"manual\"\n",
    )
    .unwrap();

    let out = bb(
        root_path.to_str().unwrap(),
        &["preflight", "demo", "--json"],
    );
    assert!(!out.status.success());
    let doc: serde_json::Value = serde_json::from_slice(&out.stdout).unwrap();
    let finding = doc["findings"]
        .as_array()
        .unwrap()
        .iter()
        .find(|f| f["kind"] == "unspawnable_binary")
        .expect("unspawnable_binary finding");
    assert!(finding["detail"]
        .as_str()
        .unwrap()
        .contains("/no/such/binary/here"));
}

#[test]
fn preflight_ok_when_secrets_and_binary_present() {
    let dir = tempfile::tempdir().unwrap();
    let root_path = dir.path();
    mkdirs(root_path, &["demo"]);
    fs::write(root_path.join("plane.toml"), "dev = true\n").unwrap();
    let stub = root_path.join("stub.sh");
    write_executable(&stub, "#!/bin/sh\ncat > /dev/null\n");
    fs::write(
        root_path.join("agents/stub.toml"),
        format!(
            "version = 1\nharness = \"command\"\nmodel = \"\"\nbin = \"{}\"\n",
            stub.display()
        ),
    )
    .unwrap();
    fs::write(
        root_path.join("tasks/demo/task.toml"),
        "agent = \"stub\"\nsubstrate = \"local\"\n[[trigger]]\nkind = \"manual\"\n",
    )
    .unwrap();

    let out = bb(
        root_path.to_str().unwrap(),
        &["preflight", "demo", "--json"],
    );
    assert!(
        out.status.success(),
        "stdout:\n{}\nstderr:\n{}",
        String::from_utf8_lossy(&out.stdout),
        String::from_utf8_lossy(&out.stderr)
    );
    let doc: serde_json::Value = serde_json::from_slice(&out.stdout).unwrap();
    assert!(doc["findings"].as_array().unwrap().is_empty());
    assert!(doc["tasks_checked"]
        .as_array()
        .unwrap()
        .contains(&serde_json::json!("demo")));
}

#[test]
fn preflight_reports_subscription_auth_probe_failure_without_creating_a_run() {
    let dir = tempfile::tempdir().unwrap();
    let root_path = dir.path();
    mkdirs(root_path, &["build"]);
    fs::write(root_path.join("plane.toml"), "dev = true\n").unwrap();
    let harness = root_path.join("codex-stub.sh");
    write_executable(&harness, "#!/bin/sh\nexit 0\n");
    fs::write(
        root_path.join("agents/codex.toml"),
        format!(
            "version = 1\nharness = \"codex\"\nmodel = \"gpt-5.5\"\nbin = \"{}\"\n",
            harness.display()
        ),
    )
    .unwrap();
    fs::write(
        root_path.join("tasks/build/task.toml"),
        "agent = \"codex\"\nsubstrate = \"sprites\"\n[workspace]\nhost = \"misty-step/lane-1\"\n[[trigger]]\nkind = \"manual\"\n",
    )
    .unwrap();
    let probe = root_path.join("auth-probe.sh");
    write_executable(
        &probe,
        "#!/bin/sh\nprintf '%s|%s|%s|%s|%s\\n' \"$BB_PREFLIGHT_TASK\" \"$BB_PREFLIGHT_HOST\" \"$BB_PREFLIGHT_HARNESS\" \"$BB_PREFLIGHT_BIN\" \"$BB_PREFLIGHT_MODEL\"\necho 'refresh_token_reused' >&2\nexit 42\n",
    );

    let out = Command::new(env!("CARGO_BIN_EXE_bb"))
        .args([
            "--config",
            root_path.to_str().unwrap(),
            "preflight",
            "build",
            "--json",
        ])
        .env(
            "BB_PREFLIGHT_SUBSCRIPTION_AUTH_PROBE_CODEX",
            probe.as_os_str(),
        )
        .output()
        .unwrap();
    assert!(!out.status.success());
    assert_eq!(out.status.code(), Some(2));
    let doc: serde_json::Value = serde_json::from_slice(&out.stdout).unwrap();
    let finding = doc["findings"]
        .as_array()
        .unwrap()
        .iter()
        .find(|f| f["kind"] == "subscription_auth_unready")
        .expect("subscription_auth_unready finding");
    assert_eq!(finding["classification"], "readiness");
    assert_eq!(finding["task"], "build");
    assert_eq!(finding["host"], "misty-step/lane-1");
    assert_eq!(finding["substrate"], "sprites");
    assert_eq!(finding["harness"], "codex");
    assert_eq!(finding["bin"], harness.to_str().unwrap());
    assert_eq!(finding["model"], "gpt-5.5");
    assert!(finding["detail"]
        .as_str()
        .unwrap()
        .contains("refresh_token_reused"));
    assert!(finding["remediation"]
        .as_str()
        .unwrap()
        .contains("codex login"));

    let plane = Plane::load(root_path).unwrap();
    let ledger = Ledger::open(&plane.db_path()).unwrap();
    assert!(ledger.list_runs(None, None).unwrap().is_empty());
}

#[test]
fn subscription_auth_dispatch_still_runs_without_preflight_interposition() {
    let dir = tempfile::tempdir().unwrap();
    let root_path = dir.path();
    mkdirs(root_path, &["demo"]);
    fs::write(root_path.join("plane.toml"), "dev = true\n").unwrap();
    let stub = root_path.join("claude-stub.sh");
    write_executable(&stub, CLAUDE_STUB);
    fs::write(
        root_path.join("agents/claude.toml"),
        format!(
            "version = 1\nharness = \"claude\"\nmodel = \"claude-fable-5\"\nbin = \"{}\"\n",
            stub.display()
        ),
    )
    .unwrap();
    fs::write(
        root_path.join("tasks/demo/task.toml"),
        "agent = \"claude\"\nsubstrate = \"local\"\n[[trigger]]\nkind = \"manual\"\n",
    )
    .unwrap();
    let plane = Plane::load(root_path).unwrap();
    let mut ledger = Ledger::open(&plane.db_path()).unwrap();

    let run_id = ingest_manual(&mut ledger, "demo");
    let run = dispatch::dispatch_run(&plane, &mut ledger, &run_id).unwrap();

    assert_eq!(run.state, "success");
    assert_eq!(ledger.list_runs(Some("demo"), None).unwrap().len(), 1);
}

#[test]
fn preflight_storm_covers_gate_required_members() {
    let dir = tempfile::tempdir().unwrap();
    let root_path = dir.path();
    mkdirs(root_path, &["verify"]);
    fs::write(
        root_path.join("plane.toml"),
        "dev = true\n[gate]\nrequired = [\"verify\"]\n",
    )
    .unwrap();
    // verify member declares verdict = "verify" but is missing a secret.
    fs::write(
        root_path.join("agents/stub.toml"),
        "version = 1\nharness = \"command\"\nmodel = \"\"\nbin = \"true\"\nsecrets = [\"BB_PREFLIGHT_STORM_MISSING\"]\n",
    )
    .unwrap();
    fs::write(
        root_path.join("tasks/verify/task.toml"),
        "agent = \"stub\"\nsubstrate = \"local\"\nverdict = \"verify\"\n[[trigger]]\nkind = \"manual\"\n",
    )
    .unwrap();

    let out = Command::new(env!("CARGO_BIN_EXE_bb"))
        .args([
            "--config",
            root_path.to_str().unwrap(),
            "preflight",
            "--storm",
            "--json",
        ])
        .env_remove("BB_PREFLIGHT_STORM_MISSING")
        .output()
        .unwrap();
    assert!(!out.status.success());
    let doc: serde_json::Value = serde_json::from_slice(&out.stdout).unwrap();
    assert!(doc["tasks_checked"]
        .as_array()
        .unwrap()
        .contains(&serde_json::json!("verify")));
    assert!(doc["findings"]
        .as_array()
        .unwrap()
        .iter()
        .any(|f| f["kind"] == "missing_secret" && f["task"] == "verify"));
}

#[test]
fn preflight_needs_a_target() {
    let dir = tempfile::tempdir().unwrap();
    let root_path = dir.path();
    mkdirs(root_path, &[]);
    fs::write(root_path.join("plane.toml"), "dev = true\n").unwrap();
    let out = bb(root_path.to_str().unwrap(), &["preflight", "--json"]);
    assert!(!out.status.success());
    let err = String::from_utf8_lossy(&out.stderr);
    assert!(
        err.contains("task") || err.contains("--storm"),
        "got: {err}"
    );
}
